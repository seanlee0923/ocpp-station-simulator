package simulator

import (
	"context"
	"strconv"
	"time"

	"github.com/seanlee0923/ocpp/protocol"
	"github.com/seanlee0923/ocpp/station"
	"github.com/seanlee0923/ocpp/v16"
)

type v16Simulator struct {
	eventBus
	station *station.Station
}

func newV16Simulator(cfg StationConfig) (Simulator, error) {
	sim := &v16Simulator{eventBus: newEventBus()}
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
	sim.station = st

	if err := station.Handle(st, sim.handleRemoteStart); err != nil {
		return nil, err
	}
	if err := station.Handle(st, sim.handleRemoteStop); err != nil {
		return nil, err
	}
	if err := station.Handle(st, sim.handleReset); err != nil {
		return nil, err
	}
	return sim, nil
}

func (sim *v16Simulator) Connect(ctx context.Context) error {
	go func() { _ = sim.station.Run(ctx) }()
	return nil
}

func (sim *v16Simulator) Disconnect() { sim.station.Stop() }

func (sim *v16Simulator) State() station.ConnectionState { return sim.station.State() }

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
	confirmation, err := callAndEmit[v16.BootNotificationRequest, v16.BootNotificationConfirmation](ctx, &sim.eventBus, sim.station, "BootNotification", req)
	if err != nil {
		return BootResult{}, err
	}
	return BootResult{Status: string(confirmation.Status), CurrentTime: confirmation.CurrentTime, Interval: confirmation.Interval}, nil
}

func (sim *v16Simulator) SendAuthorize(ctx context.Context, idTag string) (AuthorizeResult, error) {
	confirmation, err := callAndEmit[v16.AuthorizeRequest, v16.AuthorizeConfirmation](ctx, &sim.eventBus, sim.station, "Authorize", v16.AuthorizeRequest{IDTag: idTag})
	if err != nil {
		return AuthorizeResult{}, err
	}
	return AuthorizeResult{Status: string(confirmation.IDTagInfo.Status)}, nil
}

func (sim *v16Simulator) StartTransaction(ctx context.Context, req StartTxRequest) (StartTxResult, error) {
	request := v16.StartTransactionRequest{
		ConnectorID: req.ConnectorID,
		IDTag:       req.IDTag,
		MeterStart:  req.MeterStart,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	confirmation, err := callAndEmit[v16.StartTransactionRequest, v16.StartTransactionConfirmation](ctx, &sim.eventBus, sim.station, "StartTransaction", request)
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
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		TransactionID: transactionID,
		Reason:        stopReason16(req.Reason),
	}
	_, err = callAndEmit[v16.StopTransactionRequest, v16.StopTransactionConfirmation](ctx, &sim.eventBus, sim.station, "StopTransaction", request)
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
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
			SampledValue: samples,
		}},
	}
	_, err := callAndEmit[v16.MeterValuesRequest, v16.MeterValuesConfirmation](ctx, &sim.eventBus, sim.station, "MeterValues", request)
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
	request := v16.StatusNotificationRequest{
		ConnectorID: req.ConnectorID,
		ErrorCode:   errorCode,
		Status:      v16.StatusNotificationRequestStatus(req.Status),
		Info:        info,
	}
	_, err := callAndEmit[v16.StatusNotificationRequest, v16.StatusNotificationConfirmation](ctx, &sim.eventBus, sim.station, "StatusNotification", request)
	return err
}

func stopReason16(reason string) *v16.StopTransactionRequestReason {
	if reason == "" {
		return nil
	}
	value := v16.StopTransactionRequestReason(reason)
	return &value
}
