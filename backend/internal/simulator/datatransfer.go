package simulator

import "sync"

// DataTransferResult is what SendDataTransfer returns for an outbound
// DataTransfer.
type DataTransferResult struct {
	Status string `json:"status"`
	Data   string `json:"data,omitempty"`
}

type dataTransferResponse struct {
	status string
	data   string
}

// dataTransferMatcher holds operator-registered canned responses for
// inbound DataTransfer.req, keyed by (vendorID, messageID). It is embedded
// by each version adapter — station.Handle only supports one handler per
// action, so a single generic inbound handler consults this matcher instead
// of registering per-vendorId/messageId handlers (which the library has no
// way to express, since DataTransfer's shape is entirely vendor-defined).
type dataTransferMatcher struct {
	mu        sync.RWMutex
	responses map[[2]string]dataTransferResponse
}

func newDataTransferMatcher() dataTransferMatcher {
	return dataTransferMatcher{responses: make(map[[2]string]dataTransferResponse)}
}

func (m *dataTransferMatcher) register(vendorID, messageID, status, data string) {
	m.mu.Lock()
	m.responses[[2]string{vendorID, messageID}] = dataTransferResponse{status: status, data: data}
	m.mu.Unlock()
}

func (m *dataTransferMatcher) unregister(vendorID, messageID string) {
	m.mu.Lock()
	delete(m.responses, [2]string{vendorID, messageID})
	m.mu.Unlock()
}

// lookup tries an exact (vendorID, messageID) match first, then falls back
// to (vendorID, "") — a registration with no messageId matches any
// messageId for that vendor. ok is false when neither matches, in which
// case the caller should answer UnknownVendorId: this simulator has no
// concept of "known vendor, unknown message" since it never knows about a
// vendorId until the operator registers something for it.
func (m *dataTransferMatcher) lookup(vendorID, messageID string) (dataTransferResponse, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if response, ok := m.responses[[2]string{vendorID, messageID}]; ok {
		return response, true
	}
	if messageID != "" {
		if response, ok := m.responses[[2]string{vendorID, ""}]; ok {
			return response, true
		}
	}
	return dataTransferResponse{}, false
}
