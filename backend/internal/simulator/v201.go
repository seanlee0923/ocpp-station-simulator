package simulator

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/seanlee0923/ocpp/protocol"
	"github.com/seanlee0923/ocpp/station"
	"github.com/seanlee0923/ocpp/v201"
)

type v201Simulator struct {
	eventBus
	dataTransferMatcher
	configStore configStore
	// lastFirmwareRequestID/lastLogRequestID cache the requestId a CSMS sent
	// with its most recent UpdateFirmware/GetLog, so the operator-triggered
	// FirmwareStatusNotification/LogStatusNotification (see
	// SendFirmwareStatusNotification/SendDiagnosticsStatusNotification
	// below) can echo it back — the spec requires that correlation, but
	// nothing about it needs to survive a reconnect, so a plain in-memory
	// field (not persisted) is enough. -1 means "none seen yet".
	lastFirmwareRequestID atomic.Int64
	lastLogRequestID      atomic.Int64

	// mu protects cfg and runCtx — see v16Simulator's identical fields for
	// why (credential rotation via rebuilding station.Station).
	mu         sync.Mutex
	cfg        StationConfig
	runCtx     context.Context
	stationPtr atomic.Pointer[station.Station]
}

func newV201Simulator(cfg StationConfig) (Simulator, error) {
	sim := &v201Simulator{eventBus: newEventBus(), dataTransferMatcher: newDataTransferMatcher(), configStore: newConfigStore(), cfg: cfg}
	sim.lastFirmwareRequestID.Store(-1)
	sim.lastLogRequestID.Store(-1)
	st, err := sim.buildStation()
	if err != nil {
		return nil, err
	}
	sim.stationPtr.Store(st)
	return sim, nil
}

// buildStation mirrors v16Simulator.buildStation — see its doc comment.
func (sim *v201Simulator) buildStation() (*station.Station, error) {
	sim.mu.Lock()
	cfg := sim.cfg
	sim.mu.Unlock()

	st, err := station.New(station.Config{
		URL:             cfg.CSMSURL,
		Identity:        cfg.Identity,
		Version:         protocol.OCPP201,
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
		func() error { return station.Handle(st, sim.handleRequestStart) },
		func() error { return station.Handle(st, sim.handleRequestStop) },
		func() error { return station.Handle(st, sim.handleReset) },
		func() error { return station.Handle(st, sim.handleUpdateFirmware) },
		func() error { return station.Handle(st, sim.handleGetLog) },
		func() error { return station.Handle(st, sim.handleDataTransfer) },
		func() error { return station.Handle(st, sim.handleSetVariables) },
		func() error { return station.Handle(st, sim.handleGetVariables) },
	}
	for _, register := range handlers {
		if err := register(); err != nil {
			return nil, err
		}
	}
	return st, nil
}

func (sim *v201Simulator) st() *station.Station { return sim.stationPtr.Load() }

// Connect mirrors v16Simulator.Connect — see its doc comment for why every
// Connect runs a freshly built station.Station.
func (sim *v201Simulator) Connect(ctx context.Context) error {
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

func (sim *v201Simulator) Disconnect() { sim.st().Stop() }

func (sim *v201Simulator) State() station.ConnectionState { return sim.st().State() }

// rotateBasicAuthPassword mirrors v16Simulator.rotateBasicAuthPassword —
// see its doc comment.
func (sim *v201Simulator) rotateBasicAuthPassword(newPassword string) {
	sim.mu.Lock()
	sim.cfg.BasicAuthPass = newPassword
	ctx := sim.runCtx
	sim.mu.Unlock()
	if ctx == nil {
		return
	}
	if err := sim.Connect(ctx); err != nil {
		sim.emitMessage(EventMessageReceived, "SetVariables", "error", map[string]string{"error": "failed to rebuild connection: " + err.Error()})
	}
}

func (sim *v201Simulator) handleRequestStart(_ context.Context, req v201.RequestStartTransactionRequest) (v201.RequestStartTransactionConfirmation, error) {
	sim.emitRemoteCommand("RequestStartTransaction", req)
	return v201.RequestStartTransactionConfirmation{Status: v201.RequestStartTransactionConfirmationRequestStartStopStatusEnumAccepted}, nil
}

func (sim *v201Simulator) handleRequestStop(_ context.Context, req v201.RequestStopTransactionRequest) (v201.RequestStopTransactionConfirmation, error) {
	sim.emitRemoteCommand("RequestStopTransaction", req)
	return v201.RequestStopTransactionConfirmation{Status: v201.RequestStopTransactionConfirmationRequestStartStopStatusEnumAccepted}, nil
}

func (sim *v201Simulator) handleReset(_ context.Context, req v201.ResetRequest) (v201.ResetConfirmation, error) {
	sim.emitRemoteCommand("Reset", req)
	return v201.ResetConfirmation{Status: v201.ResetConfirmationResetStatusEnumAccepted}, nil
}

func (sim *v201Simulator) handleUpdateFirmware(_ context.Context, req v201.UpdateFirmwareRequest) (v201.UpdateFirmwareConfirmation, error) {
	sim.lastFirmwareRequestID.Store(int64(req.RequestID))
	sim.emitRemoteCommand("UpdateFirmware", req)
	return v201.UpdateFirmwareConfirmation{Status: v201.UpdateFirmwareConfirmationUpdateFirmwareStatusEnumAccepted}, nil
}

func (sim *v201Simulator) handleGetLog(_ context.Context, req v201.GetLogRequest) (v201.GetLogConfirmation, error) {
	sim.lastLogRequestID.Store(int64(req.RequestID))
	sim.emitRemoteCommand("GetLog", req)
	fileName := "diagnostics.log"
	return v201.GetLogConfirmation{Status: v201.GetLogConfirmationLogStatusEnumAccepted, Filename: &fileName}, nil
}

// handleDataTransfer mirrors v16Simulator.handleDataTransfer — see its doc
// comment for why one generic handler covers every vendorId/messageId.
func (sim *v201Simulator) handleDataTransfer(_ context.Context, req v201.DataTransferRequest) (v201.DataTransferConfirmation, error) {
	sim.emitRemoteCommand("DataTransfer", req)
	messageID := ""
	if req.MessageID != nil {
		messageID = *req.MessageID
	}
	response, ok := sim.lookup(req.VendorID, messageID)
	if !ok {
		return v201.DataTransferConfirmation{Status: v201.DataTransferConfirmationDataTransferStatusEnumUnknownVendorID}, nil
	}
	confirmation := v201.DataTransferConfirmation{Status: v201.DataTransferConfirmationDataTransferStatusEnum(response.status)}
	if response.data != "" {
		confirmation.Data = json.RawMessage(response.data)
	}
	return confirmation, nil
}

func (sim *v201Simulator) RegisterDataTransferResponse(vendorID, messageID, status, data string) {
	sim.register(vendorID, messageID, status, data)
}

func (sim *v201Simulator) UnregisterDataTransferResponse(vendorID, messageID string) {
	sim.unregister(vendorID, messageID)
}

// handleSetVariables stores every component/variable pair as-is (see
// variableKey). Component "SecurityCtrlr" / Variable "BasicAuthPassword" is
// 2.0.1/2.1's equivalent of 1.6's AuthorizationKey and triggers the same
// credential rotation — spawned in its own goroutine so this handler's own
// response isn't held up by the disconnect/reconnect it causes.
func (sim *v201Simulator) handleSetVariables(_ context.Context, req v201.SetVariablesRequest) (v201.SetVariablesConfirmation, error) {
	sim.emitRemoteCommand("SetVariables", req)
	results := make([]v201.SetVariablesConfirmationSetVariableResult, 0, len(req.SetVariableData))
	for _, item := range req.SetVariableData {
		componentInstance, variableInstance := "", ""
		if item.Component.Instance != nil {
			componentInstance = *item.Component.Instance
		}
		if item.Variable.Instance != nil {
			variableInstance = *item.Variable.Instance
		}
		sim.configStore.set(variableKey(item.Component.Name, componentInstance, item.Variable.Name, variableInstance), item.AttributeValue)
		if item.Component.Name == "SecurityCtrlr" && item.Variable.Name == "BasicAuthPassword" {
			go sim.rotateBasicAuthPassword(item.AttributeValue)
		}
		results = append(results, v201.SetVariablesConfirmationSetVariableResult{
			AttributeStatus: v201.SetVariablesConfirmationSetVariableStatusEnumAccepted,
			Component:       v201.SetVariablesConfirmationComponent{Name: item.Component.Name, Instance: item.Component.Instance},
			Variable:        v201.SetVariablesConfirmationVariable{Name: item.Variable.Name, Instance: item.Variable.Instance},
		})
	}
	return v201.SetVariablesConfirmation{SetVariableResult: results}, nil
}

// handleGetVariables never discloses BasicAuthPassword's value — see
// v16Simulator.handleGetConfiguration's identical reasoning for
// AuthorizationKey.
func (sim *v201Simulator) handleGetVariables(_ context.Context, req v201.GetVariablesRequest) (v201.GetVariablesConfirmation, error) {
	sim.emitRemoteCommand("GetVariables", req)
	results := make([]v201.GetVariablesConfirmationGetVariableResult, 0, len(req.GetVariableData))
	for _, item := range req.GetVariableData {
		componentInstance, variableInstance := "", ""
		if item.Component.Instance != nil {
			componentInstance = *item.Component.Instance
		}
		if item.Variable.Instance != nil {
			variableInstance = *item.Variable.Instance
		}
		result := v201.GetVariablesConfirmationGetVariableResult{
			Component: v201.GetVariablesConfirmationComponent{Name: item.Component.Name, Instance: item.Component.Instance},
			Variable:  v201.GetVariablesConfirmationVariable{Name: item.Variable.Name, Instance: item.Variable.Instance},
		}
		value, ok := sim.configStore.get(variableKey(item.Component.Name, componentInstance, item.Variable.Name, variableInstance))
		switch {
		case !ok:
			result.AttributeStatus = v201.GetVariablesConfirmationGetVariableStatusEnumUnknownVariable
		case item.Component.Name == "SecurityCtrlr" && item.Variable.Name == "BasicAuthPassword":
			result.AttributeStatus = v201.GetVariablesConfirmationGetVariableStatusEnumAccepted
		default:
			result.AttributeStatus = v201.GetVariablesConfirmationGetVariableStatusEnumAccepted
			result.AttributeValue = &value
		}
		results = append(results, result)
	}
	return v201.GetVariablesConfirmation{GetVariableResult: results}, nil
}

func (sim *v201Simulator) GetConfigValues() map[string]string { return sim.configStore.all() }

func (sim *v201Simulator) SendBootNotification(ctx context.Context, fields BootFields) (BootResult, error) {
	var serial *string
	if fields.SerialNumber != "" {
		serial = &fields.SerialNumber
	}
	var firmware *string
	if fields.FirmwareVersion != "" {
		firmware = &fields.FirmwareVersion
	}
	req := v201.BootNotificationRequest{
		ChargingStation: v201.BootNotificationRequestChargingStation{
			VendorName:      fields.VendorName,
			Model:           fields.Model,
			SerialNumber:    serial,
			FirmwareVersion: firmware,
		},
		Reason: v201.BootNotificationRequestBootReasonEnumPowerUp,
	}
	confirmation, err := callAndEmit[v201.BootNotificationRequest, v201.BootNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "BootNotification", req)
	if err != nil {
		return BootResult{}, err
	}
	return BootResult{Status: string(confirmation.Status), CurrentTime: confirmation.CurrentTime, Interval: confirmation.Interval}, nil
}

func (sim *v201Simulator) SendAuthorize(ctx context.Context, idTag string) (AuthorizeResult, error) {
	req := v201.AuthorizeRequest{IDToken: v201.AuthorizeRequestIDToken{IDToken: idTag, Type: v201.AuthorizeRequestIDTokenEnumCentral}}
	confirmation, err := callAndEmit[v201.AuthorizeRequest, v201.AuthorizeConfirmation](ctx, &sim.eventBus, sim.st(), "Authorize", req)
	if err != nil {
		return AuthorizeResult{}, err
	}
	return AuthorizeResult{Status: string(confirmation.IDTokenInfo.Status)}, nil
}

func (sim *v201Simulator) SendHeartbeat(ctx context.Context) error {
	_, err := callAndEmit[v201.HeartbeatRequest, v201.HeartbeatConfirmation](ctx, &sim.eventBus, sim.st(), "Heartbeat", v201.HeartbeatRequest{})
	return err
}

// StartTransaction generates the transaction ID itself: unlike OCPP 1.6,
// 2.0.1's TransactionEventConfirmation carries no transactionId — the
// charging station is the one that allocates it, in the request.
func (sim *v201Simulator) StartTransaction(ctx context.Context, req StartTxRequest) (StartTxResult, error) {
	transactionID := uuid.NewString()
	chargingState := v201.TransactionEventRequestChargingStateEnumCharging
	request := v201.TransactionEventRequest{
		EventType:     v201.TransactionEventRequestTransactionEventEnumStarted,
		Timestamp:     requestTimestamp(req.Timestamp),
		TriggerReason: v201.TransactionEventRequestTriggerReasonEnumAuthorized,
		SeqNo:         0,
		TransactionInfo: v201.TransactionEventRequestTransaction{
			TransactionID: transactionID,
			ChargingState: &chargingState,
		},
		EVSE:    &v201.TransactionEventRequestEVSE{ID: req.EVSEID},
		IDToken: &v201.TransactionEventRequestIDToken{IDToken: req.IDTag, Type: v201.TransactionEventRequestIDTokenEnumCentral},
	}
	_, err := callAndEmit[v201.TransactionEventRequest, v201.TransactionEventConfirmation](ctx, &sim.eventBus, sim.st(), "TransactionEvent", request)
	if err != nil {
		return StartTxResult{}, err
	}
	return StartTxResult{TransactionID: transactionID}, nil
}

func (sim *v201Simulator) StopTransaction(ctx context.Context, req StopTxRequest) error {
	stoppedReason := stopReason201(req.Reason)
	request := v201.TransactionEventRequest{
		EventType:     v201.TransactionEventRequestTransactionEventEnumEnded,
		Timestamp:     requestTimestamp(req.Timestamp),
		TriggerReason: v201.TransactionEventRequestTriggerReasonEnumStopAuthorized,
		SeqNo:         1,
		TransactionInfo: v201.TransactionEventRequestTransaction{
			TransactionID: req.TransactionID,
			StoppedReason: stoppedReason,
		},
	}
	_, err := callAndEmit[v201.TransactionEventRequest, v201.TransactionEventConfirmation](ctx, &sim.eventBus, sim.st(), "TransactionEvent", request)
	return err
}

func (sim *v201Simulator) SendMeterValues(ctx context.Context, req MeterValuesRequest) error {
	samples := make([]v201.MeterValuesRequestSampledValue, 0, len(req.Samples))
	for _, sample := range req.Samples {
		item := v201.MeterValuesRequestSampledValue{Value: parseMeterValue(sample.Value)}
		if sample.Measurand != "" {
			measurand := v201.MeterValuesRequestMeasurandEnum(sample.Measurand)
			item.Measurand = &measurand
		}
		samples = append(samples, item)
	}
	request := v201.MeterValuesRequest{
		EVSEID: req.EVSEID,
		MeterValue: []v201.MeterValuesRequestMeterValue{{
			Timestamp:    requestTimestamp(req.Timestamp),
			SampledValue: samples,
		}},
	}
	_, err := callAndEmit[v201.MeterValuesRequest, v201.MeterValuesConfirmation](ctx, &sim.eventBus, sim.st(), "MeterValues", request)
	return err
}

func (sim *v201Simulator) SendStatusNotification(ctx context.Context, req StatusRequest) error {
	request := v201.StatusNotificationRequest{
		Timestamp:       requestTimestamp(req.Timestamp),
		ConnectorStatus: v201.StatusNotificationRequestConnectorStatusEnum(req.Status),
		EVSEID:          req.EVSEID,
		ConnectorID:     req.ConnectorID,
	}
	_, err := callAndEmit[v201.StatusNotificationRequest, v201.StatusNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "StatusNotification", request)
	return err
}

func (sim *v201Simulator) SendFirmwareStatusNotification(ctx context.Context, status string) error {
	request := v201.FirmwareStatusNotificationRequest{Status: v201.FirmwareStatusNotificationRequestFirmwareStatusEnum(status)}
	if id := sim.lastFirmwareRequestID.Load(); id >= 0 {
		requestID := int(id)
		request.RequestID = &requestID
	}
	_, err := callAndEmit[v201.FirmwareStatusNotificationRequest, v201.FirmwareStatusNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "FirmwareStatusNotification", request)
	return err
}

// SendDiagnosticsStatusNotification maps to 2.0.1/2.1's LogStatusNotification
// — see the Simulator interface doc comment for why this is named after 1.6's
// concept instead of "SendLogStatusNotification".
func (sim *v201Simulator) SendDiagnosticsStatusNotification(ctx context.Context, status string) error {
	request := v201.LogStatusNotificationRequest{Status: v201.LogStatusNotificationRequestUploadLogStatusEnum(status)}
	if id := sim.lastLogRequestID.Load(); id >= 0 {
		requestID := int(id)
		request.RequestID = &requestID
	}
	_, err := callAndEmit[v201.LogStatusNotificationRequest, v201.LogStatusNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "LogStatusNotification", request)
	return err
}

func (sim *v201Simulator) SendDataTransfer(ctx context.Context, vendorID, messageID, data string) (DataTransferResult, error) {
	request := v201.DataTransferRequest{VendorID: vendorID}
	if messageID != "" {
		request.MessageID = &messageID
	}
	if data != "" {
		request.Data = json.RawMessage(data)
	}
	confirmation, err := callAndEmit[v201.DataTransferRequest, v201.DataTransferConfirmation](ctx, &sim.eventBus, sim.st(), "DataTransfer", request)
	if err != nil {
		return DataTransferResult{}, err
	}
	result := DataTransferResult{Status: string(confirmation.Status)}
	if confirmation.Data != nil {
		if raw, err := json.Marshal(confirmation.Data); err == nil {
			result.Data = string(raw)
		}
	}
	return result, nil
}

func stopReason201(reason string) *v201.TransactionEventRequestReasonEnum {
	if reason == "" {
		reason = "Local"
	}
	value := v201.TransactionEventRequestReasonEnum(reason)
	return &value
}
