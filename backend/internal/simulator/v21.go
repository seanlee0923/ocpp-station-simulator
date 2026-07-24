package simulator

import (
	"context"
	"encoding/json"
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
	station *station.Station
	// See v201Simulator's identical fields for why these exist.
	lastFirmwareRequestID atomic.Int64
	lastLogRequestID      atomic.Int64
}

func newV21Simulator(cfg StationConfig) (Simulator, error) {
	sim := &v21Simulator{eventBus: newEventBus(), dataTransferMatcher: newDataTransferMatcher()}
	sim.lastFirmwareRequestID.Store(-1)
	sim.lastLogRequestID.Store(-1)
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
	sim.station = st

	if err := station.Handle(st, sim.handleRequestStart); err != nil {
		return nil, err
	}
	if err := station.Handle(st, sim.handleRequestStop); err != nil {
		return nil, err
	}
	if err := station.Handle(st, sim.handleReset); err != nil {
		return nil, err
	}
	if err := station.Handle(st, sim.handleUpdateFirmware); err != nil {
		return nil, err
	}
	if err := station.Handle(st, sim.handleGetLog); err != nil {
		return nil, err
	}
	if err := station.Handle(st, sim.handleDataTransfer); err != nil {
		return nil, err
	}
	return sim, nil
}

func (sim *v21Simulator) Connect(ctx context.Context) error {
	go func() { _ = sim.station.Run(ctx) }()
	return nil
}

func (sim *v21Simulator) Disconnect() { sim.station.Stop() }

func (sim *v21Simulator) State() station.ConnectionState { return sim.station.State() }

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
	confirmation, err := callAndEmit[v21.BootNotificationRequest, v21.BootNotificationConfirmation](ctx, &sim.eventBus, sim.station, "BootNotification", req)
	if err != nil {
		return BootResult{}, err
	}
	return BootResult{Status: string(confirmation.Status), CurrentTime: confirmation.CurrentTime, Interval: confirmation.Interval}, nil
}

func (sim *v21Simulator) SendAuthorize(ctx context.Context, idTag string) (AuthorizeResult, error) {
	req := v21.AuthorizeRequest{IDToken: v21.AuthorizeRequestIDToken{IDToken: idTag, Type: "Central"}}
	confirmation, err := callAndEmit[v21.AuthorizeRequest, v21.AuthorizeConfirmation](ctx, &sim.eventBus, sim.station, "Authorize", req)
	if err != nil {
		return AuthorizeResult{}, err
	}
	return AuthorizeResult{Status: string(confirmation.IDTokenInfo.Status)}, nil
}

// StartTransaction generates the transaction ID itself: unlike OCPP 1.6,
// 2.1's TransactionEventConfirmation carries no transactionId — the
// charging station is the one that allocates it, in the request.
func (sim *v21Simulator) StartTransaction(ctx context.Context, req StartTxRequest) (StartTxResult, error) {
	transactionID := uuid.NewString()
	chargingState := v21.TransactionEventRequestChargingStateEnumCharging
	request := v21.TransactionEventRequest{
		EventType:     v21.TransactionEventRequestTransactionEventEnumStarted,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		TriggerReason: v21.TransactionEventRequestTriggerReasonEnumAuthorized,
		SeqNo:         0,
		TransactionInfo: v21.TransactionEventRequestTransaction{
			TransactionID: transactionID,
			ChargingState: &chargingState,
		},
		EVSE:    &v21.TransactionEventRequestEVSE{ID: req.EVSEID},
		IDToken: &v21.TransactionEventRequestIDToken{IDToken: req.IDTag, Type: "Central"},
	}
	_, err := callAndEmit[v21.TransactionEventRequest, v21.TransactionEventConfirmation](ctx, &sim.eventBus, sim.station, "TransactionEvent", request)
	if err != nil {
		return StartTxResult{}, err
	}
	return StartTxResult{TransactionID: transactionID}, nil
}

func (sim *v21Simulator) StopTransaction(ctx context.Context, req StopTxRequest) error {
	stoppedReason := stopReason21(req.Reason)
	request := v21.TransactionEventRequest{
		EventType:     v21.TransactionEventRequestTransactionEventEnumEnded,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		TriggerReason: v21.TransactionEventRequestTriggerReasonEnumStopAuthorized,
		SeqNo:         1,
		TransactionInfo: v21.TransactionEventRequestTransaction{
			TransactionID: req.TransactionID,
			StoppedReason: stoppedReason,
		},
	}
	_, err := callAndEmit[v21.TransactionEventRequest, v21.TransactionEventConfirmation](ctx, &sim.eventBus, sim.station, "TransactionEvent", request)
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
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
			SampledValue: samples,
		}},
	}
	_, err := callAndEmit[v21.MeterValuesRequest, v21.MeterValuesConfirmation](ctx, &sim.eventBus, sim.station, "MeterValues", request)
	return err
}

func (sim *v21Simulator) SendStatusNotification(ctx context.Context, req StatusRequest) error {
	request := v21.StatusNotificationRequest{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		ConnectorStatus: v21.StatusNotificationRequestConnectorStatusEnum(req.Status),
		EVSEID:          req.EVSEID,
		ConnectorID:     req.ConnectorID,
	}
	_, err := callAndEmit[v21.StatusNotificationRequest, v21.StatusNotificationConfirmation](ctx, &sim.eventBus, sim.station, "StatusNotification", request)
	return err
}

func (sim *v21Simulator) SendFirmwareStatusNotification(ctx context.Context, status string) error {
	request := v21.FirmwareStatusNotificationRequest{Status: v21.FirmwareStatusNotificationRequestFirmwareStatusEnum(status)}
	if id := sim.lastFirmwareRequestID.Load(); id >= 0 {
		requestID := int(id)
		request.RequestID = &requestID
	}
	_, err := callAndEmit[v21.FirmwareStatusNotificationRequest, v21.FirmwareStatusNotificationConfirmation](ctx, &sim.eventBus, sim.station, "FirmwareStatusNotification", request)
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
	_, err := callAndEmit[v21.LogStatusNotificationRequest, v21.LogStatusNotificationConfirmation](ctx, &sim.eventBus, sim.station, "LogStatusNotification", request)
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
	confirmation, err := callAndEmit[v21.DataTransferRequest, v21.DataTransferConfirmation](ctx, &sim.eventBus, sim.station, "DataTransfer", request)
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
