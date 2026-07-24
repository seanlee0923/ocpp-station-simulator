package api

import (
	"sync"

	"gorm.io/gorm"

	"ocpp-station-simulator/backend/internal/db"
)

// eventWriter persists db.StationEvent rows off the hot path. It mirrors
// the fixed-worker-pool, drop-when-full pattern reviewed in ocpp-go's
// csms.Metrics dispatch: a slow or down database must never be able to
// stall the OCPP connection whose activity produced the event.
type eventWriter struct {
	queue chan db.StationEvent
	stop  chan struct{}
	wg    sync.WaitGroup
}

const (
	eventQueueSize = 1024
	eventWorkers   = 8
)

func newEventWriter(database *gorm.DB) *eventWriter {
	w := &eventWriter{queue: make(chan db.StationEvent, eventQueueSize), stop: make(chan struct{})}
	for range eventWorkers {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			for {
				select {
				case row := <-w.queue:
					database.Create(&row)
				case <-w.stop:
					return
				}
			}
		}()
	}
	return w
}

func (w *eventWriter) enqueue(row db.StationEvent) {
	select {
	case w.queue <- row:
	default:
	}
}

func (w *eventWriter) Close() {
	close(w.stop)
	w.wg.Wait()
}
