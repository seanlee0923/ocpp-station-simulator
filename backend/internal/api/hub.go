package api

import "sync"

// Hub fans out already-JSON-encoded event payloads to every browser
// WebSocket currently watching a given station. It is intentionally
// separate from simulator.Registry: the registry knows nothing about
// WebSockets, and the hub knows nothing about OCPP.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan []byte]struct{})}
}

const subscriberBufferSize = 128

func (h *Hub) Subscribe(stationID string) chan []byte {
	ch := make(chan []byte, subscriberBufferSize)
	h.mu.Lock()
	if h.subs[stationID] == nil {
		h.subs[stationID] = make(map[chan []byte]struct{})
	}
	h.subs[stationID][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(stationID string, ch chan []byte) {
	h.mu.Lock()
	if subs := h.subs[stationID]; subs != nil {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(h.subs, stationID)
		}
	}
	h.mu.Unlock()
	close(ch)
}

// Broadcast never blocks: a subscriber whose buffer is already full is
// slower than the browser can render anyway, so this drops the message for
// that one subscriber rather than stalling every station's event dispatch.
func (h *Hub) Broadcast(stationID string, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[stationID] {
		select {
		case ch <- payload:
		default:
		}
	}
}
