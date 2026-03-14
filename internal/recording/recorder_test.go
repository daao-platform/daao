package recording

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// RecordingWriter tests
// ============================================================================

func TestRecordingWriter_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	w, err := NewRecordingWriter("rec-1", "sess-1", dir, 80, 24, "test")
	if err != nil {
		t.Fatalf("NewRecordingWriter failed: %v", err)
	}
	defer w.Close()

	filename := filepath.Join(dir, "rec-1.cast")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatal("expected .cast file to exist")
	}
}

func TestRecordingWriter_HeaderIsValid(t *testing.T) {
	dir := t.TempDir()
	w, err := NewRecordingWriter("rec-2", "sess-2", dir, 120, 40, "Title Test")
	if err != nil {
		t.Fatalf("NewRecordingWriter failed: %v", err)
	}
	w.Close()

	data, err := os.ReadFile(filepath.Join(dir, "rec-2.cast"))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 1 {
		t.Fatal("expected at least 1 line (header)")
	}

	header := lines[0]
	if !strings.Contains(header, `"version":2`) {
		t.Errorf("header missing version:2, got: %s", header)
	}
	if !strings.Contains(header, `"width":120`) {
		t.Errorf("header missing width:120, got: %s", header)
	}
	if !strings.Contains(header, `"height":40`) {
		t.Errorf("header missing height:40, got: %s", header)
	}
	if !strings.Contains(header, `"title":"Title Test"`) {
		t.Errorf("header missing title, got: %s", header)
	}
}

func TestRecordingWriter_WriteAndClose(t *testing.T) {
	dir := t.TempDir()
	w, err := NewRecordingWriter("rec-3", "sess-3", dir, 80, 24, "")
	if err != nil {
		t.Fatalf("NewRecordingWriter failed: %v", err)
	}

	w.Write([]byte("hello"))
	w.Write([]byte(" world"))
	durationMs, sizeBytes := w.Close()

	if durationMs < 0 {
		t.Errorf("expected non-negative duration, got %d", durationMs)
	}
	if sizeBytes != 11 { // "hello" (5) + " world" (6)
		t.Errorf("expected sizeBytes=11, got %d", sizeBytes)
	}

	// Read file and verify events
	data, _ := os.ReadFile(filepath.Join(dir, "rec-3.cast"))
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 { // header + 2 events
		t.Errorf("expected 3 lines, got %d", len(lines))
	}

	// Each event line should contain "o" and the data
	if !strings.Contains(lines[1], `"o"`) {
		t.Errorf("event line missing type: %s", lines[1])
	}
	if !strings.Contains(lines[1], `"hello"`) {
		t.Errorf("event line missing data: %s", lines[1])
	}
	if !strings.Contains(lines[2], `" world"`) {
		t.Errorf("event line missing data: %s", lines[2])
	}
}

func TestRecordingWriter_WriteAfterClose(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewRecordingWriter("rec-4", "sess-4", dir, 80, 24, "")

	w.Write([]byte("before"))
	w.Close()
	w.Write([]byte("after")) // should be a no-op

	data, _ := os.ReadFile(filepath.Join(dir, "rec-4.cast"))
	if strings.Contains(string(data), "after") {
		t.Error("write after close should be silently ignored")
	}
}

func TestRecordingWriter_EmptyWrite(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewRecordingWriter("rec-5", "sess-5", dir, 80, 24, "")

	w.Write([]byte{}) // should be a no-op
	w.Write(nil)      // should be a no-op
	_, sizeBytes := w.Close()

	if sizeBytes != 0 {
		t.Errorf("expected sizeBytes=0, got %d", sizeBytes)
	}
}

func TestRecordingWriter_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewRecordingWriter("rec-6", "sess-6", dir, 80, 24, "")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Write([]byte("x"))
		}()
	}
	wg.Wait()
	_, sizeBytes := w.Close()

	if sizeBytes != 100 {
		t.Errorf("expected sizeBytes=100, got %d", sizeBytes)
	}
}

func TestRecordingWriter_CloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewRecordingWriter("rec-7", "sess-7", dir, 80, 24, "")

	w.Write([]byte("data"))
	_, s1 := w.Close()
	d2, s2 := w.Close()

	if s1 == 0 {
		t.Error("first close should return non-zero sizeBytes")
	}
	if d2 != 0 || s2 != 0 {
		t.Errorf("second close should return zero, got d=%d s=%d", d2, s2)
	}
}

func TestRecordingWriter_Filename(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewRecordingWriter("my-recording", "sess", dir, 80, 24, "")
	defer w.Close()

	if w.Filename() != "my-recording.cast" {
		t.Errorf("expected 'my-recording.cast', got '%s'", w.Filename())
	}
}

// ============================================================================
// RecordingPool tests
// ============================================================================

func TestRecordingPool_StartStop(t *testing.T) {
	dir := t.TempDir()
	pool := NewRecordingPool(dir)

	err := pool.StartRecording("sess-a", "rec-a", 80, 24, "test")
	if err != nil {
		t.Fatalf("StartRecording failed: %v", err)
	}

	if !pool.IsRecording("sess-a") {
		t.Error("expected IsRecording=true")
	}

	// Write some data
	pool.WriteIfRecording("sess-a", []byte("test data"))

	dur, size, err := pool.StopRecording("sess-a")
	if err != nil {
		t.Fatalf("StopRecording failed: %v", err)
	}
	if dur < 0 {
		t.Errorf("expected non-negative duration, got %d", dur)
	}
	if size != 9 {
		t.Errorf("expected size=9, got %d", size)
	}

	if pool.IsRecording("sess-a") {
		t.Error("expected IsRecording=false after stop")
	}
}

func TestRecordingPool_DoubleStart(t *testing.T) {
	dir := t.TempDir()
	pool := NewRecordingPool(dir)

	pool.StartRecording("sess-b", "rec-b1", 80, 24, "")
	err := pool.StartRecording("sess-b", "rec-b2", 80, 24, "")
	if err == nil {
		t.Error("expected error on double start")
	}
	pool.StopRecording("sess-b")
}

func TestRecordingPool_StopNonExistent(t *testing.T) {
	dir := t.TempDir()
	pool := NewRecordingPool(dir)

	_, _, err := pool.StopRecording("nonexistent")
	if err == nil {
		t.Error("expected error on stop non-existent")
	}
}

func TestRecordingPool_WriteIfRecording_NoOp(t *testing.T) {
	dir := t.TempDir()
	pool := NewRecordingPool(dir)

	// Should not panic or error
	pool.WriteIfRecording("nonexistent", []byte("data"))
}

func TestRecordingPool_StopAll(t *testing.T) {
	dir := t.TempDir()
	pool := NewRecordingPool(dir)

	pool.StartRecording("s1", "r1", 80, 24, "")
	pool.StartRecording("s2", "r2", 80, 24, "")
	pool.WriteIfRecording("s1", []byte("data1"))
	pool.WriteIfRecording("s2", []byte("data2"))

	pool.StopAll()

	if pool.IsRecording("s1") || pool.IsRecording("s2") {
		t.Error("expected all recordings stopped")
	}
}

func TestRecordingPool_DataDir(t *testing.T) {
	pool := NewRecordingPool("/data/recordings")
	if pool.DataDir() != "/data/recordings" {
		t.Errorf("expected '/data/recordings', got '%s'", pool.DataDir())
	}
}

// ============================================================================
// RecordingPlayer tests
// ============================================================================

func TestRecordingPlayer_ParseAndEvents(t *testing.T) {
	dir := t.TempDir()

	// Create a recording with known data
	w, _ := NewRecordingWriter("rec-play", "sess-play", dir, 80, 24, "Player Test")
	w.Write([]byte("line1\n"))
	time.Sleep(10 * time.Millisecond)
	w.Write([]byte("line2\n"))
	w.Close()

	// Parse it with the player
	p, err := NewRecordingPlayer(filepath.Join(dir, "rec-play.cast"))
	if err != nil {
		t.Fatalf("NewRecordingPlayer failed: %v", err)
	}

	if p.Header() == nil {
		t.Fatal("expected non-nil header")
	}
	if p.Header().Version != 2 {
		t.Errorf("expected version=2, got %d", p.Header().Version)
	}
	if p.Header().Width != 80 {
		t.Errorf("expected width=80, got %d", p.Header().Width)
	}
	if p.Header().Title != "Player Test" {
		t.Errorf("expected title='Player Test', got '%s'", p.Header().Title)
	}

	events := p.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if string(events[0].Data) != "line1\n" {
		t.Errorf("expected 'line1\\n', got '%s'", events[0].Data)
	}
	if string(events[1].Data) != "line2\n" {
		t.Errorf("expected 'line2\\n', got '%s'", events[1].Data)
	}

	// Second event should have a later offset
	if events[1].Offset <= events[0].Offset {
		t.Error("expected event offsets to increase")
	}
}

func TestRecordingPlayer_Duration(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewRecordingWriter("rec-dur", "sess-dur", dir, 80, 24, "")
	w.Write([]byte("a"))
	time.Sleep(50 * time.Millisecond)
	w.Write([]byte("b"))
	w.Close()

	p, _ := NewRecordingPlayer(filepath.Join(dir, "rec-dur.cast"))
	dur := p.Duration()
	if dur < 40 || dur > 500 { // should be ~50ms, allow margin
		t.Errorf("expected duration ~50ms, got %dms", dur)
	}
}

func TestRecordingPlayer_EmptyRecording(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewRecordingWriter("rec-empty", "sess-empty", dir, 80, 24, "")
	w.Close()

	p, _ := NewRecordingPlayer(filepath.Join(dir, "rec-empty.cast"))
	if len(p.Events()) != 0 {
		t.Errorf("expected 0 events, got %d", len(p.Events()))
	}
	if p.Duration() != 0 {
		t.Errorf("expected 0 duration, got %d", p.Duration())
	}
}

func TestRecordingPlayer_StreamEvents(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewRecordingWriter("rec-stream", "sess-stream", dir, 80, 24, "")
	w.Write([]byte("event1"))
	w.Write([]byte("event2"))
	w.Write([]byte("event3"))
	w.Close()

	p, _ := NewRecordingPlayer(filepath.Join(dir, "rec-stream.cast"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use 100x speed to make the test fast
	ch := p.StreamEvents(ctx, 100.0)

	var received []string
	for evt := range ch {
		received = append(received, string(evt.Data))
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 events, got %d", len(received))
	}
	if received[0] != "event1" || received[1] != "event2" || received[2] != "event3" {
		t.Errorf("unexpected event data: %v", received)
	}
}

func TestRecordingPlayer_StreamEventsCancellation(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewRecordingWriter("rec-cancel", "sess-cancel", dir, 80, 24, "")
	for i := 0; i < 100; i++ {
		w.Write([]byte("x"))
		time.Sleep(time.Millisecond)
	}
	w.Close()

	p, _ := NewRecordingPlayer(filepath.Join(dir, "rec-cancel.cast"))

	ctx, cancel := context.WithCancel(context.Background())

	// Use 0.01x speed (very slow) so cancellation happens before all events
	ch := p.StreamEvents(ctx, 0.01)

	// Read one event then cancel
	<-ch
	cancel()

	// Drain remaining — should stop quickly
	count := 0
	for range ch {
		count++
	}

	// We shouldn't get all 100 events (only the ones already buffered)
	if count >= 100 {
		t.Errorf("expected fewer than 100 events after cancel, got %d", count)
	}
}

func TestRecordingPlayer_NonexistentFile(t *testing.T) {
	_, err := NewRecordingPlayer("/nonexistent/file.cast")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}
