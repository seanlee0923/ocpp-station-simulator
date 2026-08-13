package simulator

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/seanlee0923/ocpp/station"
)

var ErrNotFound = errors.New("station not found")

// ManagedStation pairs a runtime Simulator with the bookkeeping the registry
// needs to stop it cleanly.
type ManagedStation struct {
	ID     string
	Config StationConfig
	Sim    Simulator

	cancel          context.CancelFunc
	done            chan struct{}
	heartbeatCancel context.CancelFunc
}

// Registry holds every currently-connecting-or-connected virtual station in
// this process. It is the runtime reflection of whichever db.Station rows
// the operator has chosen to bring up; a process restart starts with an
// empty Registry (see plan: no auto-reconnect on restart, by design).
type Registry struct {
	mu       sync.RWMutex
	stations map[string]*ManagedStation
	// onEvent forwards every Simulator event to the DB writer and WS hub.
	// Injected so this package stays free of any api/db dependency.
	onEvent func(stationID string, event Event)
}

func NewRegistry(onEvent func(stationID string, event Event)) *Registry {
	return &Registry{stations: make(map[string]*ManagedStation), onEvent: onEvent}
}

// Create constructs the version-appropriate Simulator for id/cfg, registers
// it, and starts connecting. The returned error is a construction failure
// (bad version, station.New rejecting cfg); a connection failure afterward
// surfaces asynchronously as an EventDisconnected on the station's event
// feed, matching how station.Run itself reports connection outcomes.
func (r *Registry) Create(id string, cfg StationConfig) (*ManagedStation, error) {
	sim, err := New(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	managed := &ManagedStation{ID: id, Config: cfg, Sim: sim, cancel: cancel, done: make(chan struct{})}

	r.mu.Lock()
	r.stations[id] = managed
	r.mu.Unlock()

	go r.forward(id, sim, managed.done)
	r.setHeartbeat(managed, cfg.HeartbeatInterval)

	if err := sim.Connect(ctx); err != nil {
		cancel()
		r.mu.Lock()
		delete(r.stations, id)
		r.mu.Unlock()
		close(managed.done)
		return nil, err
	}
	return managed, nil
}

// heartbeat sends an OCPP Heartbeat only while the station is connected.
// It belongs to the managed station lifecycle, so disconnect merely pauses
// delivery and reconnect resumes it without creating a duplicate ticker.
func (r *Registry) heartbeat(managed *ManagedStation, ctx context.Context, interval int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if managed.Sim.State() != station.Connected {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_ = managed.Sim.SendHeartbeat(ctx)
			cancel()
		case <-ctx.Done():
			return
		}
	}
}

func (r *Registry) setHeartbeat(managed *ManagedStation, interval int) {
	if managed.heartbeatCancel != nil {
		managed.heartbeatCancel()
		managed.heartbeatCancel = nil
	}
	managed.Config.HeartbeatInterval = interval
	if interval <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	managed.heartbeatCancel = cancel
	go r.heartbeat(managed, ctx, interval)
}

// SetHeartbeatInterval replaces the running ticker immediately. Zero stops
// automatic Heartbeat calls without disconnecting the station.
func (r *Registry) SetHeartbeatInterval(id string, interval int) error {
	if interval < 0 {
		return errors.New("heartbeat interval must not be negative")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	managed, ok := r.stations[id]
	if !ok {
		return ErrNotFound
	}
	r.setHeartbeat(managed, interval)
	return nil
}

// forward relays sim's events until done is closed (station removed). It
// deliberately never closes sim.Events() itself — the channel is simply
// abandoned and garbage-collected once nothing references sim anymore.
func (r *Registry) forward(id string, sim Simulator, done <-chan struct{}) {
	for {
		select {
		case event := <-sim.Events():
			if r.onEvent != nil {
				r.onEvent(id, event)
			}
		case <-done:
			return
		}
	}
}

func (r *Registry) Get(id string) (*ManagedStation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	managed, ok := r.stations[id]
	if !ok {
		return nil, ErrNotFound
	}
	return managed, nil
}

func (r *Registry) List() []*ManagedStation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*ManagedStation, 0, len(r.stations))
	for _, managed := range r.stations {
		result = append(result, managed)
	}
	return result
}

// Disconnect stops the station's OCPP connection but keeps it registered
// (used by the /disconnect API action, as opposed to Delete which removes
// it entirely).
func (r *Registry) Disconnect(id string) error {
	managed, err := r.Get(id)
	if err != nil {
		return err
	}
	managed.Sim.Disconnect()
	return nil
}

// Reconnect restarts a previously disconnected station's Connect loop with
// a fresh cancelable context.
func (r *Registry) Reconnect(id string) error {
	managed, err := r.Get(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	managed.cancel = cancel
	return managed.Sim.Connect(ctx)
}

func (r *Registry) Delete(id string) error {
	r.mu.Lock()
	managed, ok := r.stations[id]
	if !ok {
		r.mu.Unlock()
		return ErrNotFound
	}
	delete(r.stations, id)
	r.mu.Unlock()

	managed.Sim.Disconnect()
	if managed.heartbeatCancel != nil {
		managed.heartbeatCancel()
	}
	managed.cancel()
	close(managed.done)
	return nil
}
