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
	"github.com/seanlee0923/ocpp/v21"
)

type v21Simulator struct {
	eventBus
	dataTransferMatcher
	configStore configStore
	// See v201Simulator's identical fields for why these exist.
	lastFirmwareRequestID atomic.Int64
	lastLogRequestID      atomic.Int64

	mu         sync.Mutex
	cfg        StationConfig
	runCtx     context.Context
	stationPtr atomic.Pointer[station.Station]
}

func newV21Simulator(cfg StationConfig) (Simulator, error) {
	sim := &v21Simulator{eventBus: newEventBus(), dataTransferMatcher: newDataTransferMatcher(), configStore: newConfigStore(), cfg: cfg}
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
func (sim *v21Simulator) buildStation() (*station.Station, error) {
	sim.mu.Lock()
	cfg := sim.cfg
	sim.mu.Unlock()

	st, err := station.New(station.Config{
		URL:             cfg.CSMSURL,
		Identity:        cfg.Identity,
		Version:         protocol.OCPP21,
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

func (sim *v21Simulator) st() *station.Station { return sim.stationPtr.Load() }

// Connect mirrors v16Simulator.Connect — see its doc comment for why every
// Connect runs a freshly built station.Station.
func (sim *v21Simulator) Connect(ctx context.Context) error {
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

func (sim *v21Simulator) Disconnect() { sim.st().Stop() }

func (sim *v21Simulator) State() station.ConnectionState { return sim.st().State() }

// rotateBasicAuthPassword mirrors v16Simulator.rotateBasicAuthPassword —
// see its doc comment.
func (sim *v21Simulator) rotateBasicAuthPassword(newPassword string) {
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

func (sim *v21Simulator) handleRequestStart(_ context.Context, req v21.RequestStartTransactionRequest) (v21.RequestStartTransactionConfirmation, error) {
	sim.emitRemoteCommand("RequestStartTransaction", req)
	return v21.RequestStartTransactionConfirmation{Status: v21.RequestStartTransactionConfirmationRequestStartStopStatusEnumAccepted}, nil
}

func (sim *v21Simulator) handleRequestStop(_ context.Context, req v21.RequestStopTransactionRequest) (v21.RequestStopTransactionConfirmation, error) {
	sim.emitRemoteCommand("RequestStopTransaction", req)
	return v21.RequestStopTransactionConfirmation{Status: v21.RequestStopTransactionConfirmationRequestStartStopStatusEnumAccepted}, nil
}

func (sim *v21Simulator) handleReset(_ context.Context, req v21.ResetRequest) (v21.ResetConfirmation, error) {
	sim.emitRemoteCommand("Reset", req)
	return v21.ResetConfirmation{Status: v21.ResetConfirmationResetStatusEnumAccepted}, nil
}

func (sim *v21Simulator) handleUpdateFirmware(_ context.Context, req v21.UpdateFirmwareRequest) (v21.UpdateFirmwareConfirmation, error) {
	sim.lastFirmwareRequestID.Store(int64(req.RequestID))
	sim.emitRemoteCommand("UpdateFirmware", req)
	return v21.UpdateFirmwareConfirmation{Status: v21.UpdateFirmwareConfirmationUpdateFirmwareStatusEnumAccepted}, nil
}

func (sim *v21Simulator) handleGetLog(_ context.Context, req v21.GetLogRequest) (v21.GetLogConfirmation, error) {
	sim.lastLogRequestID.Store(int64(req.RequestID))
	sim.emitRemoteCommand("GetLog", req)
	fileName := "diagnostics.log"
	return v21.GetLogConfirmation{Status: v21.GetLogConfirmationLogStatusEnumAccepted, Filename: &fileName}, nil
}

// handleDataTransfer mirrors v16Simulator.handleDataTransfer — see its doc
// comment for why one generic handler covers every vendorId/messageId.
func (sim *v21Simulator) handleDataTransfer(_ context.Context, req v21.DataTransferRequest) (v21.DataTransferConfirmation, error) {
	sim.emitRemoteCommand("DataTransfer", req)
	messageID := ""
	if req.MessageID != nil {
		messageID = *req.MessageID
	}
	response, ok := sim.lookup(req.VendorID, messageID)
	if !ok {
		return v21.DataTransferConfirmation{Status: v21.DataTransferConfirmationDataTransferStatusEnumUnknownVendorID}, nil
	}
	confirmation := v21.DataTransferConfirmation{Status: v21.DataTransferConfirmationDataTransferStatusEnum(response.status)}
	if response.data != "" {
		confirmation.Data = json.RawMessage(response.data)
	}
	return confirmation, nil
}

func (sim *v21Simulator) RegisterDataTransferResponse(vendorID, messageID, status, data string) {
	sim.register(vendorID, messageID, status, data)
}

func (sim *v21Simulator) UnregisterDataTransferResponse(vendorID, messageID string) {
	sim.unregister(vendorID, messageID)
}

// handleSetVariables mirrors v201Simulator.handleSetVariables — see its doc
// comment.
func (sim *v21Simulator) handleSetVariables(_ context.Context, req v21.SetVariablesRequest) (v21.SetVariablesConfirmation, error) {
	sim.emitRemoteCommand("SetVariables", req)
	results := make([]v21.SetVariablesConfirmationSetVariableResult, 0, len(req.SetVariableData))
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
		results = append(results, v21.SetVariablesConfirmationSetVariableResult{
			AttributeStatus: v21.SetVariablesConfirmationSetVariableStatusEnumAccepted,
			Component:       v21.SetVariablesConfirmationComponent{Name: item.Component.Name, Instance: item.Component.Instance},
			Variable:        v21.SetVariablesConfirmationVariable{Name: item.Variable.Name, Instance: item.Variable.Instance},
		})
	}
	return v21.SetVariablesConfirmation{SetVariableResult: results}, nil
}

// handleGetVariables mirrors v201Simulator.handleGetVariables — see its doc
// comment for why BasicAuthPassword's value is never disclosed.
func (sim *v21Simulator) handleGetVariables(_ context.Context, req v21.GetVariablesRequest) (v21.GetVariablesConfirmation, error) {
	sim.emitRemoteCommand("GetVariables", req)
	results := make([]v21.GetVariablesConfirmationGetVariableResult, 0, len(req.GetVariableData))
	for _, item := range req.GetVariableData {
		componentInstance, variableInstance := "", ""
		if item.Component.Instance != nil {
			componentInstance = *item.Component.Instance
		}
		if item.Variable.Instance != nil {
			variableInstance = *item.Variable.Instance
		}
		result := v21.GetVariablesConfirmationGetVariableResult{
			Component: v21.GetVariablesConfirmationComponent{Name: item.Component.Name, Instance: item.Component.Instance},
			Variable:  v21.GetVariablesConfirmationVariable{Name: item.Variable.Name, Instance: item.Variable.Instance},
		}
		value, ok := sim.configStore.get(variableKey(item.Component.Name, componentInstance, item.Variable.Name, variableInstance))
		switch {
		case !ok:
			result.AttributeStatus = v21.GetVariablesConfirmationGetVariableStatusEnumUnknownVariable
		case item.Component.Name == "SecurityCtrlr" && item.Variable.Name == "BasicAuthPassword":
			result.AttributeStatus = v21.GetVariablesConfirmationGetVariableStatusEnumAccepted
		default:
			result.AttributeStatus = v21.GetVariablesConfirmationGetVariableStatusEnumAccepted
			result.AttributeValue = &value
		}
		results = append(results, result)
	}
	return v21.GetVariablesConfirmation{GetVariableResult: results}, nil
}

func (sim *v21Simulator) GetConfigValues() map[string]string { return sim.configStore.all() }

func (sim *v21Simulator) SendBootNotification(ctx context.Context, fields BootFields) (BootResult, error) {
	var serial *string
	if fields.SerialNumber != "" {
		serial = &fields.SerialNumber
	}
	var firmware *string
	if fields.FirmwareVersion != "" {
		firmware = &fields.FirmwareVersion
	}
	req := v21.BootNotificationRequest{
		ChargingStation: v21.BootNotificationRequestChargingStation{
			VendorName:      fields.VendorName,
			Model:           fields.Model,
			SerialNumber:    serial,
			FirmwareVersion: firmware,
		},
		Reason: v21.BootNotificationRequestBootReasonEnumPowerUp,
	}
	confirmation, err := callAndEmit[v21.BootNotificationRequest, v21.BootNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "BootNotification", req)
	if err != nil {
		return BootResult{}, err
	}
	return BootResult{Status: string(confirmation.Status), CurrentTime: confirmation.CurrentTime, Interval: confirmation.Interval}, nil
}

func (sim *v21Simulator) SendAuthorize(ctx context.Context, idTag string) (AuthorizeResult, error) {
	req := v21.AuthorizeRequest{IDToken: v21.AuthorizeRequestIDToken{IDToken: idTag, Type: "Central"}}
	confirmation, err := callAndEmit[v21.AuthorizeRequest, v21.AuthorizeConfirmation](ctx, &sim.eventBus, sim.st(), "Authorize", req)
	if err != nil {
		return AuthorizeResult{}, err
	}
	return AuthorizeResult{Status: string(confirmation.IDTokenInfo.Status)}, nil
}

func (sim *v21Simulator) SendHeartbeat(ctx context.Context) error {
	_, err := callAndEmit[v21.HeartbeatRequest, v21.HeartbeatConfirmation](ctx, &sim.eventBus, sim.st(), "Heartbeat", v21.HeartbeatRequest{})
	return err
}

// StartTransaction generates the transaction ID itself: unlike OCPP 1.6,
// 2.1's TransactionEventConfirmation carries no transactionId — the
// charging station is the one that allocates it, in the request.
func (sim *v21Simulator) StartTransaction(ctx context.Context, req StartTxRequest) (StartTxResult, error) {
	transactionID := uuid.NewString()
	chargingState := v21.TransactionEventRequestChargingStateEnumCharging
	request := v21.TransactionEventRequest{
		EventType:     v21.TransactionEventRequestTransactionEventEnumStarted,
		Timestamp:     requestTimestamp(req.Timestamp),
		TriggerReason: v21.TransactionEventRequestTriggerReasonEnumAuthorized,
		SeqNo:         0,
		TransactionInfo: v21.TransactionEventRequestTransaction{
			TransactionID: transactionID,
			ChargingState: &chargingState,
		},
		EVSE:    &v21.TransactionEventRequestEVSE{ID: req.EVSEID},
		IDToken: &v21.TransactionEventRequestIDToken{IDToken: req.IDTag, Type: "Central"},
	}
	_, err := callAndEmit[v21.TransactionEventRequest, v21.TransactionEventConfirmation](ctx, &sim.eventBus, sim.st(), "TransactionEvent", request)
	if err != nil {
		return StartTxResult{}, err
	}
	return StartTxResult{TransactionID: transactionID}, nil
}

func (sim *v21Simulator) StopTransaction(ctx context.Context, req StopTxRequest) error {
	stoppedReason := stopReason21(req.Reason)
	request := v21.TransactionEventRequest{
		EventType:     v21.TransactionEventRequestTransactionEventEnumEnded,
		Timestamp:     requestTimestamp(req.Timestamp),
		TriggerReason: v21.TransactionEventRequestTriggerReasonEnumStopAuthorized,
		SeqNo:         1,
		TransactionInfo: v21.TransactionEventRequestTransaction{
			TransactionID: req.TransactionID,
			StoppedReason: stoppedReason,
		},
	}
	_, err := callAndEmit[v21.TransactionEventRequest, v21.TransactionEventConfirmation](ctx, &sim.eventBus, sim.st(), "TransactionEvent", request)
	return err
}

func (sim *v21Simulator) SendMeterValues(ctx context.Context, req MeterValuesRequest) error {
	samples := make([]v21.MeterValuesRequestSampledValue, 0, len(req.Samples))
	for _, sample := range req.Samples {
		item := v21.MeterValuesRequestSampledValue{Value: parseMeterValue(sample.Value)}
		if sample.Measurand != "" {
			measurand := v21.MeterValuesRequestMeasurandEnum(sample.Measurand)
			item.Measurand = &measurand
		}
		samples = append(samples, item)
	}
	request := v21.MeterValuesRequest{
		EVSEID: req.EVSEID,
		MeterValue: []v21.MeterValuesRequestMeterValue{{
			Timestamp:    requestTimestamp(req.Timestamp),
			SampledValue: samples,
		}},
	}
	_, err := callAndEmit[v21.MeterValuesRequest, v21.MeterValuesConfirmation](ctx, &sim.eventBus, sim.st(), "MeterValues", request)
	return err
}

func (sim *v21Simulator) SendStatusNotification(ctx context.Context, req StatusRequest) error {
	request := v21.StatusNotificationRequest{
		Timestamp:       requestTimestamp(req.Timestamp),
		ConnectorStatus: v21.StatusNotificationRequestConnectorStatusEnum(req.Status),
		EVSEID:          req.EVSEID,
		ConnectorID:     req.ConnectorID,
	}
	_, err := callAndEmit[v21.StatusNotificationRequest, v21.StatusNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "StatusNotification", request)
	return err
}

func (sim *v21Simulator) SendFirmwareStatusNotification(ctx context.Context, status string) error {
	request := v21.FirmwareStatusNotificationRequest{Status: v21.FirmwareStatusNotificationRequestFirmwareStatusEnum(status)}
	if id := sim.lastFirmwareRequestID.Load(); id >= 0 {
		requestID := int(id)
		request.RequestID = &requestID
	}
	_, err := callAndEmit[v21.FirmwareStatusNotificationRequest, v21.FirmwareStatusNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "FirmwareStatusNotification", request)
	return err
}

// SendDiagnosticsStatusNotification maps to 2.1's LogStatusNotification —
// see the Simulator interface doc comment for why this is named after 1.6's
// concept instead of "SendLogStatusNotification".
func (sim *v21Simulator) SendDiagnosticsStatusNotification(ctx context.Context, status string) error {
	request := v21.LogStatusNotificationRequest{Status: v21.LogStatusNotificationRequestUploadLogStatusEnum(status)}
	if id := sim.lastLogRequestID.Load(); id >= 0 {
		requestID := int(id)
		request.RequestID = &requestID
	}
	_, err := callAndEmit[v21.LogStatusNotificationRequest, v21.LogStatusNotificationConfirmation](ctx, &sim.eventBus, sim.st(), "LogStatusNotification", request)
	return err
}

func (sim *v21Simulator) SendDataTransfer(ctx context.Context, vendorID, messageID, data string) (DataTransferResult, error) {
	request := v21.DataTransferRequest{VendorID: vendorID}
	if messageID != "" {
		request.MessageID = &messageID
	}
	if data != "" {
		request.Data = json.RawMessage(data)
	}
	confirmation, err := callAndEmit[v21.DataTransferRequest, v21.DataTransferConfirmation](ctx, &sim.eventBus, sim.st(), "DataTransfer", request)
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

func stopReason21(reason string) *v21.TransactionEventRequestReasonEnum {
	if reason == "" {
		reason = "Local"
	}
	value := v21.TransactionEventRequestReasonEnum(reason)
	return &value
}
