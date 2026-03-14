package database

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const batchFlushInterval = 100 * time.Millisecond

type pendingEvent struct {
	RunID     uuid.UUID
	EventType string
	Payload   []byte
	Sequence  int
}

type runBuffer struct {
	events []pendingEvent
	timer  *time.Timer
}

// BatchEventWriter accumulates agent run events and flushes them in bulk
// using pgx.Batch every 100ms. Reduces DB writes ~10x during token streaming.
type BatchEventWriter struct {
	pool    *pgxpool.Pool
	mu      sync.Mutex
	buffers map[uuid.UUID]*runBuffer
}

func NewBatchEventWriter(pool *pgxpool.Pool) *BatchEventWriter {
	return &BatchEventWriter{
		pool:    pool,
		buffers: make(map[uuid.UUID]*runBuffer),
	}
}

// Append adds an event to the per-run buffer and (re)starts the 100ms flush timer.
func (w *BatchEventWriter) Append(runID uuid.UUID, eventType string, payload []byte, sequence int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	buf, ok := w.buffers[runID]
	if !ok {
		buf = &runBuffer{}
		w.buffers[runID] = buf
	}

	buf.events = append(buf.events, pendingEvent{
		RunID: runID, EventType: eventType,
		Payload: payload, Sequence: sequence,
	})

	// Reset the flush timer
	if buf.timer != nil {
		buf.timer.Stop()
	}
	buf.timer = time.AfterFunc(batchFlushInterval, func() {
		w.flush(runID)
	})
}

// Flush sends all buffered events for a run to the DB immediately.
// Call this on agent_end to ensure no events are lost.
func (w *BatchEventWriter) Flush(runID uuid.UUID) {
	w.flush(runID)
}

// Close flushes and removes the buffer for a run.
// Safe to call multiple times - Flush after Close is a no-op (no panic).
func (w *BatchEventWriter) Close(runID uuid.UUID) {
	events := w.takeEvents(runID)
	if len(events) == 0 {
		return
	}
	w.executeBatch(runID, events)
}

// takeEvents atomically takes all events from the buffer and deletes the buffer.
// Must be called while holding w.mu.
func (w *BatchEventWriter) takeEvents(runID uuid.UUID) []pendingEvent {
	w.mu.Lock()
	defer w.mu.Unlock()

	buf, ok := w.buffers[runID]
	if !ok {
		return nil
	}
	if buf.timer != nil {
		buf.timer.Stop()
	}
	events := buf.events
	buf.events = buf.events[:0] // reset slice, keep capacity
	delete(w.buffers, runID)
	return events
}

func (w *BatchEventWriter) flush(runID uuid.UUID) {
	events := w.flushInternal(runID)
	if len(events) > 0 {
		w.executeBatch(runID, events)
	}
}

// flushInternal takes events from buffer without deleting it.
// Must be called while holding w.mu.
func (w *BatchEventWriter) flushInternal(runID uuid.UUID) []pendingEvent {
	w.mu.Lock()
	defer w.mu.Unlock()

	buf, ok := w.buffers[runID]
	if !ok || len(buf.events) == 0 {
		return nil
	}
	if buf.timer != nil {
		buf.timer.Stop()
	}
	events := buf.events
	buf.events = buf.events[:0] // reset slice, keep capacity
	return events
}

// BufferLen returns the number of buffered events for a run (for testing).
func (w *BatchEventWriter) BufferLen(runID uuid.UUID) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if buf, ok := w.buffers[runID]; ok {
		return len(buf.events)
	}
	return 0
}

func (w *BatchEventWriter) executeBatch(runID uuid.UUID, events []pendingEvent) {
	// No-op if pool is nil (allows testing without DB)
	if w.pool == nil {
		return
	}

	// Build pgx batch
	batch := &pgx.Batch{}
	const q = `INSERT INTO agent_run_events (run_id, event_type, payload, sequence)
			   VALUES ($1, $2, $3, $4)`
	for _, e := range events {
		batch.Queue(q, e.RunID, e.EventType, e.Payload, e.Sequence)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	br := w.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range events {
		if _, err := br.Exec(); err != nil {
			slog.Error("BatchEventWriter: batch insert failed", "run_id", runID, "err", err, "component", "db")
		}
	}
	slog.Debug("BatchEventWriter: flushed events", "run_id", runID, "count", len(events), "component", "db")
}
