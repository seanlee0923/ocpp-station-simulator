package simulator

import (
	"context"
	"strconv"

	"github.com/seanlee0923/ocpp/protocol"
	"github.com/seanlee0923/ocpp/station"
)

// callAndEmit sends req as an outbound Call and emits a message_sent /
// message_received (or an error) event around it, so every version adapter
// gets message-log coverage for free instead of hand-rolling it per action.
func callAndEmit[Req protocol.Payload, Conf protocol.Payload](
	ctx context.Context, bus *eventBus, st *station.Station, action string, req Req,
) (Conf, error) {
	bus.emitMessage(EventMessageSent, action, "sent", req)
	confirmation, err := station.Call[Req, Conf](ctx, st, req)
	if err != nil {
		// Payload must always be valid JSON (or empty) — every consumer
		// (the API layer's json.RawMessage passthrough, the frontend) relies
		// on that, so a plain err.Error() string here would corrupt the
		// response the moment this event is read back after being persisted.
		bus.emitMessage(EventMessageReceived, action, "error", map[string]string{"error": err.Error()})
		return confirmation, err
	}
	bus.emitMessage(EventMessageReceived, action, "received", confirmation)
	return confirmation, nil
}

// parseMeterValue is lenient: an operator-typed value that fails to parse as
// a number becomes 0 rather than rejecting the whole request, since this is
// a test tool, not a metering device.
func parseMeterValue(value string) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}
