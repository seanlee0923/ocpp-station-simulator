package simulator

import "sync"

// configStore holds every key/value a CSMS has set via 1.6's
// ChangeConfiguration or 2.0.1/2.1's SetVariables, for the lifetime of the
// process. There is no DB table backing this: every ChangeConfiguration/
// SetVariables call is already recorded as a StationEvent (via
// emitRemoteCommand) for audit purposes, and "current value" only needs to
// survive as long as the connection itself does — consistent with this
// app's existing choice not to auto-reconnect stations across a restart.
type configStore struct {
	mu     sync.RWMutex
	values map[string]string
}

func newConfigStore() configStore {
	return configStore{values: make(map[string]string)}
}

func (c *configStore) set(key, value string) {
	c.mu.Lock()
	c.values[key] = value
	c.mu.Unlock()
}

func (c *configStore) get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.values[key]
	return value, ok
}

func (c *configStore) all() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]string, len(c.values))
	for key, value := range c.values {
		out[key] = value
	}
	return out
}

// variableKey composes a 2.0.1/2.1 Component+Variable address into the flat
// string configStore is keyed by, since 1.6's ChangeConfiguration and 2.x's
// SetVariables share the same underlying store despite addressing config
// differently. Instance suffixes are only appended when present.
func variableKey(componentName, componentInstance, variableName, variableInstance string) string {
	component := componentName
	if componentInstance != "" {
		component += "." + componentInstance
	}
	variable := variableName
	if variableInstance != "" {
		variable += "." + variableInstance
	}
	return component + "/" + variable
}
