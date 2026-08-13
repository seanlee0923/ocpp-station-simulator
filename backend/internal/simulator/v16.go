package simulator

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/seanlee0923/ocpp/protocol"
	"github.com/seanlee0923/ocpp/station"
	"github.com/seanlee0923/ocpp/v16"
)

type v16Simulator struct {
	eventBus
	dataTransferMatcher
	configStore configStore

	// mu protects cfg and runCtx, both mutated only by a credential
	// rotation (see rotateBasicAuthPassword). stationPtr is separate and
	// lock-free (atomic.Pointer) since every SendXxx/handleXxx method reads
	// it on the hot path and a rotation must be able to swap it without
	// blocking them.
	mu         sync.Mutex
	cfg        StationConfig
	runCtx     context.Context
	stationPtr atomic.Pointer[station.Station]
}

func newV16Simulator(cfg StationConfig) (Simulator, error) {
	sim := &v16Simulator{eventBus: newEventBus(), dataTransferMatcher: newDataTransferMatcher(), configStore: newConfigStore(), cfg: cfg}
	st, err := sim.buildStation()
	if err != nil {
		return nil, err
	}
	sim.stationPtr.Store(st)
	return sim, nil
}

// buildStation creates a fresh station.Station from sim.cfg and registers
// every inbound handler on it. It's used both at construction and by
// rotateBasicAuthPassword — station.Config (including BasicAuth) is
// immutable once built, so rotating credentials means building a whole new
// instance, and both paths must register the exact same handler set or a
// rotation would silently lose handlers.
func (sim *v16Simulator) buildStation() (*station.Station, error) {
	sim.mu.Lock()
	cfg := sim.cfg
	sim.mu.Unlock()

	st, err := station.New(station.Config{
		URL:             cfg.CSMSURL,
		Identity:        cfg.Identity,
		Version:         protocol.OCPP16,
		BasicAuth:       basicAuth(cfg),
		TLSConfig:       tlsConfig(cfg),
		ReconnectPolicy: &station.ReconnectPolicy{InitialDelay: time.Second, MaxDelay: 30 * time.Second, Multiplier: 2},
		OnConnect:       func(*station.Station) { sim.emit(Event{Type: EventConnected}) },
		OnDisconnect:    func(*station.Station, error) { sim.emit(Event{Type: EventDisconnected}) },
	})
	if err != nil {
		return nil, err
	}
	handlers := []func() error{
		func() error { return station.Handle(st, sim.handleRemoteStart) },
		func() error { return station.Handle(st, sim.handleRemoteStop) },
		func() error { return station.Handle(st, sim.handleReset) },
		func() error { return station.Handle(st, sim.handleUpdateFirmware) },
		func() error { return station.Handle(st, sim.handleGetDiagnostics) },
		func() error { return station.Handle(st, sim.handleDataTransfer) },
		func() error { return station.Handle(st, sim.handleChangeConfiguration) },
		func() error { return station.Handle(st, sim.handleGetConfiguration) },
	}
	for _, register := range handlers {
		if err := register(); err != nil {
			return nil, err
		}
	}
	return st, nil
}

func (sim *v16Simulator) st() *station.Station { return sim.stationPtr.Load() }

// Connect always runs a freshly built station.Station rather than reusing
// the current one: station.Stop() is permanent (it closes an internal
// channel behind a sync.Once), so a Station that has been stopped by
// Disconnect returns ErrStopped from Run without ever dialing. Swapping in
// a new instance also means a second Connect without an intervening
// Disconnect can't leave two Run loops racing over the same identity.
func (sim *v16Simulator) Connect(ctx context.Context) error {
	newStation, err := sim.buildStation()
	if err != nil {
		return err
	}
	sim.mu.Lock()
	sim.runCtx = ctx
	sim.mu.Unlock()
	if old := sim.stationPtr.Swap(newStation); old != nil {
		old.Stop()
	}
	go func() { _ = newStation.Run(ctx) }()
	return nil
}

func (sim *v16Simulator) Disconnect() { sim.st().Stop() }

func (sim *v16Simulator) State() station.ConnectionState { return sim.st().State() }

// rotateBasicAuthPassword implements the AuthorizationKey side effect a real
// charge point has: build a new station.Station with the updated password,
// swap it in, stop the old one, and reconnect using the context Connect was
// originally called with. Runs in its own goroutine (see
// handleChangeConfiguration) so the CALLRESULT for the triggering
// ChangeConfiguration isn't held up by a disconnect/reconnect cycle.
func (sim *v16Simulator) rotateBasicAuthPassword(newPassword string) {
	sim.mu.Lock()
	sim.cfg.BasicAuthPass = newPassword
	ctx := sim.runCtx
	sim.mu.Unlock()
	if ctx == nil {
		return // never connected yet; the new password takes effect whenever Connect() first runs
	}
	if err := sim.Connect(ctx); err != nil {
		sim.emitMessage(EventMessageReceived, "ChangeConfiguration", "error", map[string]string{"error": "failed to rebuild connection: " + err.Error()})
	}
}

// handleRemoteStart, handleRemoteStop, and handleReset always answer
// Accepted: a real charge point typically acknowledges a remote command
// immediately and only then acts on it. The operator drives the actual
// StartTransaction/StopTransaction call from the UI after seeing this event
// — that mirrors real hardware and keeps a human in control of the
// simulated charging session instead of the simulator silently completing
// the whole flow on its own.
func (sim *v16Simulator) handleRemoteStart(_ context.Context, req v16.RemoteStartTransactionRequest) (v16.RemoteStartTransactionConfirmation, error) {
	sim.emitRemoteCommand("RemoteStartTransaction", req)
	return v16.RemoteStartTransactionConfirmation{Status: v16.RemoteStartTransactionConfirmationStatusAccepted}, nil
}

func (sim *v16Simulator) handleRemoteStop(_ context.Context, req v16.RemoteStopTransactionRequest) (v16.RemoteStopTransactionConfirmation, error) {
	sim.emitRemoteCommand("RemoteStopTransaction", req)
	return v16.RemoteStopTransactionConfirmation{Status: v16.RemoteStopTransactionConfirmationStatusAccepted}, nil
}

func (sim *v16Simulator) handleReset(_ context.Context, req v16.ResetRequest) (v16.ResetConfirmation, error) {
	sim.emitRemoteCommand("Reset", req)
	return v16.ResetConfirmation{Status: v16.ResetConfirmationStatusAccepted}, nil
}

// handleUpdateFirmware and handleGetDiagnostics accept unconditionally (1.6
// gives neither confirmation a status field — accepting *is* the only
// response) and just log the request. The operator reports progress
// afterward via SendFirmwareStatusNotification/SendDiagnosticsStatusNotification.
func (sim *v16Simulator) handleUpdateFirmware(_ context.Context, req v16.UpdateFirmwareRequest) (v16.UpdateFirmwareConfirmation, error) {
	sim.emitRemoteCommand("UpdateFirmware", req)
	return v16.UpdateFirmwareConfirmation{}, nil
}

func (sim *v16Simulator) handleGetDiagnostics(_ context.Context, req v16.GetDiagnosticsRequest) (v16.GetDiagnosticsConfirmation, error) {
	sim.emitRemoteCommand("GetDiagnostics", req)
	fileName := "diagnostics.zip"
	return v16.GetDiagnosticsConfirmation{FileName: &fileName}, nil
}

// handleDataTransfer is the one inbound handler covering every
// vendorId/messageId combination: DataTransfer's payload is entirely
// vendor-defined, so station.Handle (one handler per action) can't register
// per-vendorId handlers the way the schema-driven actions do. It consults
// dataTransferMatcher instead — see RegisterDataTransferResponse.
func (sim *v16Simulator) handleDataTransfer(_ context.Context, req v16.DataTransferRequest) (v16.DataTransferConfirmation, error) {
	sim.emitRemoteCommand("DataTransfer", req)
	messageID := ""
	if req.MessageID != nil {
		messageID = *req.MessageID
	}
	response, ok := sim.lookup(req.VendorID, messageID)
	if !ok {
		return v16.DataTransferConfirmation{Status: v16.DataTransferConfirmationStatusUnknownVendorID}, nil
	}
	confirmation := v16.DataTransferConfirmation{Status: v16.DataTransferConfirmationStatus(response.status)}
	if response.data != "" {
		confirmation.Data = &response.data
	}
	return confirmation, nil
}

func (sim *v16Simulator) RegisterDataTransferResponse(vendorID, messageID, status, data string) {
	sim.register(vendorID, messageID, status, data)
}

func (sim *v16Simulator) UnregisterDataTransferResponse(vendorID, messageID string) {
	sim.unregister(vendorID, messageID)
}

// handleChangeConfiguration stores every key as-is. AuthorizationKey
// specifically also triggers a credential rotation (see
// rotateBasicAuthPassword) — spawned in its own goroutine so this handler's
// own Accepted response isn't held up by the disconnect/reconnect it causes.
func (sim *v16Simulator) handleChangeConfiguration(_ context.Context, req v16.ChangeConfigurationRequest) (v16.ChangeConfigurationConfirmation, error) {
	sim.emitRemoteCommand("ChangeConfiguration", req)
	sim.configStore.set(req.Key, req.Value)
	if req.Key == "AuthorizationKey" {
		go sim.rotateBasicAuthPassword(req.Value)
	}
	return v16.ChangeConfigurationConfirmation{Status: v16.ChangeConfigurationConfirmationStatusAccepted}, nil
}

// handleGetConfiguration never discloses AuthorizationKey's value — real
// charge points treat it as write-only so the password never travels back
// over the (already-authenticated, but still) wire.
func (sim *v16Simulator) handleGetConfiguration(_ context.Context, req v16.GetConfigurationRequest) (v16.GetConfigurationConfirmation, error) {
	sim.emitRemoteCommand("GetConfiguration", req)
	keys := req.Key
	if len(keys) == 0 {
		for key := range sim.configStore.all() {
			keys = append(keys, key)
		}
	}
	var confirmation v16.GetConfigurationConfirmation
	for _, key := range keys {
		value, ok := sim.configStore.get(key)
		if !ok {
			confirmation.UnknownKey = append(confirmation.UnknownKey, key)
			continue
		}
		item := v16.GetConfigurationConfirmationConfigurationKeyItem{Key: key}
		if key != "AuthorizationKey" {
			item.Value = &value
		}
		confirmation.ConfigurationKey = append(confirmation.ConfigurationKey, item)
	}
	return confirmation, nil
}

func (sim *v16Simulator) GetConfigValues() map[string]string { return sim.configStore.all() }

func (sim *v16Simulator) SendBootNotification(ctx context.Context, fields BootFields) (BootResult, error) {
	var serial *string
	if fields.SerialNumber != "" {
		serial = &fields.SerialNumber
	}
	var firmware *string
	if fields.FirmwareVersion != "" {
		firmware = &fields.FirmwareVersion
	}
	req := v16.BootNotificationRequest{
		ChargePointVendor:       fields.VendorName,
		ChargePointModel:        fields.Model,
		ChargePointSerialNumber: serial,
		FirmwareVersion:         firmware,
	}
	confirmation, err := callAndEmit[v16.BootNotificationRequest, v16.BootNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "BootNotification", req)
	if err != nil {
		return BootResult{}, err
	}
	return BootResult{Status: string(confirmation.Status), CurrentTime: confirmation.CurrentTime, Interval: confirmation.Interval}, nil
}

func (sim *v16Simulator) SendAuthorize(ctx context.Context, idTag string) (AuthorizeResult, error) {
	confirmation, err := callAndEmit[v16.AuthorizeRequest, v16.AuthorizeConfirmation](ctx, &sim.eventBus, sim.st(), "Authorize", v16.AuthorizeRequest{IDTag: idTag})
	if err != nil {
		return AuthorizeResult{}, err
	}
	return AuthorizeResult{Status: string(confirmation.IDTagInfo.Status)}, nil
}

func (sim *v16Simulator) SendHeartbeat(ctx context.Context) error {
	_, err := callAndEmit[v16.HeartbeatRequest, v16.HeartbeatConfirmation](ctx, &sim.eventBus, sim.st(), "Heartbeat", v16.HeartbeatRequest{})
	return err
}

func (sim *v16Simulator) StartTransaction(ctx context.Context, req StartTxRequest) (StartTxResult, error) {
	request := v16.StartTransactionRequest{
		ConnectorID: req.ConnectorID,
		IDTag:       req.IDTag,
		MeterStart:  req.MeterStart,
		Timestamp:   requestTimestamp(req.Timestamp),
	}
	confirmation, err := callAndEmit[v16.StartTransactionRequest, v16.StartTransactionConfirmation](ctx, &sim.eventBus, sim.st(), "StartTransaction", request)
	if err != nil {
		return StartTxResult{}, err
	}
	return StartTxResult{TransactionID: strconv.Itoa(confirmation.TransactionID)}, nil
}

func (sim *v16Simulator) StopTransaction(ctx context.Context, req StopTxRequest) error {
	transactionID, err := strconv.Atoi(req.TransactionID)
	if err != nil {
		return err
	}
	request := v16.StopTransactionRequest{
		MeterStop:     req.MeterStop,
		Timestamp:     requestTimestamp(req.Timestamp),
		TransactionID: transactionID,
		Reason:        stopReason16(req.Reason),
	}
	_, err = callAndEmit[v16.StopTransactionRequest, v16.StopTransactionConfirmation](ctx, &sim.eventBus, sim.st(), "StopTransaction", request)
	return err
}

func (sim *v16Simulator) SendMeterValues(ctx context.Context, req MeterValuesRequest) error {
	samples := make([]v16.MeterValuesRequestMeterValueItemSampledValueItem, 0, len(req.Samples))
	for _, sample := range req.Samples {
		item := v16.MeterValuesRequestMeterValueItemSampledValueItem{Value: sample.Value}
		if sample.Measurand != "" {
			measurand := v16.MeterValuesRequestMeterValueItemSampledValueItemMeasurand(sample.Measurand)
			item.Measurand = &measurand
		}
		if sample.Unit != "" {
			unit := v16.MeterValuesRequestMeterValueItemSampledValueItemUnit(sample.Unit)
			item.Unit = &unit
		}
		samples = append(samples, item)
	}
	var transactionID *int
	if req.TransactionID != "" {
		if parsed, err := strconv.Atoi(req.TransactionID); err == nil {
			transactionID = &parsed
		}
	}
	request := v16.MeterValuesRequest{
		ConnectorID:   req.ConnectorID,
		TransactionID: transactionID,
		MeterValue: []v16.MeterValuesRequestMeterValueItem{{
			Timestamp:    requestTimestamp(req.Timestamp),
			SampledValue: samples,
		}},
	}
	_, err := callAndEmit[v16.MeterValuesRequest, v16.MeterValuesConfirmation](ctx, &sim.eventBus, sim.st(), "MeterValues", request)
	return err
}

func (sim *v16Simulator) SendStatusNotification(ctx context.Context, req StatusRequest) error {
	errorCode := v16.StatusNotificationRequestErrorCodeNoError
	if req.ErrorCode != "" {
		errorCode = v16.StatusNotificationRequestErrorCode(req.ErrorCode)
	}
	var info *string
	if req.Info != "" {
		info = &req.Info
	}
	timestamp := requestTimestamp(req.Timestamp)
	request := v16.StatusNotificationRequest{
		ConnectorID: req.ConnectorID,
		ErrorCode:   errorCode,
		Status:      v16.StatusNotificationRequestStatus(req.Status),
		Info:        info,
		Timestamp:   &timestamp,
	}
	_, err := callAndEmit[v16.StatusNotificationRequest, v16.StatusNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "StatusNotification", request)
	return err
}

func (sim *v16Simulator) SendFirmwareStatusNotification(ctx context.Context, status string) error {
	request := v16.FirmwareStatusNotificationRequest{Status: v16.FirmwareStatusNotificationRequestStatus(status)}
	_, err := callAndEmit[v16.FirmwareStatusNotificationRequest, v16.FirmwareStatusNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "FirmwareStatusNotification", request)
	return err
}

func (sim *v16Simulator) SendDiagnosticsStatusNotification(ctx context.Context, status string) error {
	request := v16.DiagnosticsStatusNotificationRequest{Status: v16.DiagnosticsStatusNotificationRequestStatus(status)}
	_, err := callAndEmit[v16.DiagnosticsStatusNotificationRequest, v16.DiagnosticsStatusNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "DiagnosticsStatusNotification", request)
	return err
}

func (sim *v16Simulator) SendDataTransfer(ctx context.Context, vendorID, messageID, data string) (DataTransferResult, error) {
	request := v16.DataTransferRequest{VendorID: vendorID}
	if messageID != "" {
		request.MessageID = &messageID
	}
	if data != "" {
		request.Data = &data
	}
	confirmation, err := callAndEmit[v16.DataTransferRequest, v16.DataTransferConfirmation](ctx, &sim.eventBus, sim.st(), "DataTransfer", request)
	if err != nil {
		return DataTransferResult{}, err
	}
	result := DataTransferResult{Status: string(confirmation.Status)}
	if confirmation.Data != nil {
		result.Data = *confirmation.Data
	}
	return result, nil
}

func stopReason16(reason string) *v16.StopTransactionRequestReason {
	if reason == "" {
		return nil
	}
	value := v16.StopTransactionRequestReason(reason)
	return &value
}
