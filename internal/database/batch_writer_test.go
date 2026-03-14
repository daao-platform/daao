package database_test

import (
	"sync"
	"testing"

	"github.com/daao/nexus/internal/database"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestBatchEventWriter_AppendAccumulates(t *testing.T) {
	// Test that 10 Appends without triggering the timer accumulate correctly
	w := database.NewBatchEventWriter(nil) // nil pool — won't actually flush
	runID := uuid.New()
	for i := 0; i < 10; i++ {
		w.Append(runID, "message_update", []byte(`{}`), i)
	}
	assert.Equal(t, 10, w.BufferLen(runID))
}

func TestBatchEventWriter_CloseFlushesAndRemoves(t *testing.T) {
	// After Close, BufferLen returns 0
	w := database.NewBatchEventWriter(nil)
	runID := uuid.New()
	w.Append(runID, "message_update", []byte(`{}`), 1)
	w.Close(runID) // flush is a no-op with nil pool, but should not panic
	assert.Equal(t, 0, w.BufferLen(runID))
}

func TestBatchEventWriter_NilPoolNocrash(t *testing.T) {
	// nil pool does not panic on Append or Flush
	w := database.NewBatchEventWriter(nil)
	runID := uuid.New()
	w.Append(runID, "message_update", []byte(`{}`), 1)
	w.Flush(runID) // should not panic
}

func TestBatchEventWriter_ConcurrentAppend(t *testing.T) {
	// Concurrent Appends are safe (race detector)
	w := database.NewBatchEventWriter(nil)
	runID := uuid.New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w.Append(runID, "message_update", []byte(`{}`), i)
		}(i)
	}
	wg.Wait()
	assert.Equal(t, 50, w.BufferLen(runID))
}

func TestBatchEventWriter_MultipleRuns(t *testing.T) {
	// Independent buffers per run
	run1, run2 := uuid.New(), uuid.New()
	w := database.NewBatchEventWriter(nil)
	w.Append(run1, "message_update", []byte(`{}`), 1)
	w.Append(run1, "message_update", []byte(`{}`), 2)
	w.Append(run2, "message_update", []byte(`{}`), 1)
	assert.Equal(t, 2, w.BufferLen(run1))
	assert.Equal(t, 1, w.BufferLen(run2))
}
