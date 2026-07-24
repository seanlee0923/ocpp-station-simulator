package simulator

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/seanlee0923/ocpp/protocol"
	"github.com/seanlee0923/ocpp/station"
	"github.com/seanlee0923/ocpp/v201"
)

type v201Simulator struct {
	eventBus
	station *station.Station
}

func newV201Simulator(cfg StationConfig) (Simulator, error) {
	sim := &v201Simulator{eventBus: newEventBus()}
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
	return sim, nil
}

func (sim *v201Simulator) Connect(ctx context.Context) error {
	go func() { _ = sim.station.Run(ctx) }()
	return nil
}

func (sim *v201Simulator) Disconnect() { sim.station.Stop() }

func (sim *v201Simulator) State() station.ConnectionState { return sim.station.State() }

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
	confirmation, err := callAndEmit[v201.BootNotificationRequest, v201.BootNotificationConfirmation](ctx, &sim.eventBus, sim.station, "BootNotification", req)
	if err != nil {
		return BootResult{}, err
	}
	return BootResult{Status: string(confirmation.Status), CurrentTime: confirmation.CurrentTime, Interval: confirmation.Interval}, nil
}

func (sim *v201Simulator) SendAuthorize(ctx context.Context, idTag string) (AuthorizeResult, error) {
	req := v201.AuthorizeRequest{IDToken: v201.AuthorizeRequestIDToken{IDToken: idTag, Type: v201.AuthorizeRequestIDTokenEnumCentral}}
	confirmation, err := callAndEmit[v201.AuthorizeRequest, v201.AuthorizeConfirmation](ctx, &sim.eventBus, sim.station, "Authorize", req)
	if err != nil {
		return AuthorizeResult{}, err
	}
	return AuthorizeResult{Status: string(confirmation.IDTokenInfo.Status)}, nil
}

// StartTransaction generates the transaction ID itself: unlike OCPP 1.6,
// 2.0.1's TransactionEventConfirmation carries no transactionId — the
// charging station is the one that allocates it, in the request.
func (sim *v201Simulator) StartTransaction(ctx context.Context, req StartTxRequest) (StartTxResult, error) {
	transactionID := uuid.NewString()
	chargingState := v201.TransactionEventRequestChargingStateEnumCharging
	request := v201.TransactionEventRequest{
		EventType:     v201.TransactionEventRequestTransactionEventEnumStarted,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		TriggerReason: v201.TransactionEventRequestTriggerReasonEnumAuthorized,
		SeqNo:         0,
		TransactionInfo: v201.TransactionEventRequestTransaction{
			TransactionID: transactionID,
			ChargingState: &chargingState,
		},
		EVSE:    &v201.TransactionEventRequestEVSE{ID: req.EVSEID},
		IDToken: &v201.TransactionEventRequestIDToken{IDToken: req.IDTag, Type: v201.TransactionEventRequestIDTokenEnumCentral},
	}
	_, err := callAndEmit[v201.TransactionEventRequest, v201.TransactionEventConfirmation](ctx, &sim.eventBus, sim.station, "TransactionEvent", request)
	if err != nil {
		return StartTxResult{}, err
	}
	return StartTxResult{TransactionID: transactionID}, nil
}

func (sim *v201Simulator) StopTransaction(ctx context.Context, req StopTxRequest) error {
	stoppedReason := stopReason201(req.Reason)
	request := v201.TransactionEventRequest{
		EventType:     v201.TransactionEventRequestTransactionEventEnumEnded,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		TriggerReason: v201.TransactionEventRequestTriggerReasonEnumStopAuthorized,
		SeqNo:         1,
		TransactionInfo: v201.TransactionEventRequestTransaction{
			TransactionID: req.TransactionID,
			StoppedReason: stoppedReason,
		},
	}
	_, err := callAndEmit[v201.TransactionEventRequest, v201.TransactionEventConfirmation](ctx, &sim.eventBus, sim.station, "TransactionEvent", request)
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
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
			SampledValue: samples,
		}},
	}
	_, err := callAndEmit[v201.MeterValuesRequest, v201.MeterValuesConfirmation](ctx, &sim.eventBus, sim.station, "MeterValues", request)
	return err
}

func (sim *v201Simulator) SendStatusNotification(ctx context.Context, req StatusRequest) error {
	request := v201.StatusNotificationRequest{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		ConnectorStatus: v201.StatusNotificationRequestConnectorStatusEnum(req.Status),
		EVSEID:          req.EVSEID,
		ConnectorID:     req.ConnectorID,
	}
	_, err := callAndEmit[v201.StatusNotificationRequest, v201.StatusNotificationConfirmation](ctx, &sim.eventBus, sim.station, "StatusNotification", request)
	return err
}

func stopReason201(reason string) *v201.TransactionEventRequestReasonEnum {
	if reason == "" {
		reason = "Local"
	}
	value := v201.TransactionEventRequestReasonEnum(reason)
	return &value
}
