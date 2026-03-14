package recording

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// RecordingEvent represents a single event in a recording.
type RecordingEvent struct {
	Offset float64 // seconds since recording start
	Type   string  // "o" = output
	Data   []byte  // raw terminal bytes
}

// RecordingPlayer reads and streams events from an asciicast v2 (.cast) file.
type RecordingPlayer struct {
	filePath string
	header   *asciicastHeader
	events   []RecordingEvent
}

// NewRecordingPlayer creates a player for the given .cast file.
func NewRecordingPlayer(filePath string) (*RecordingPlayer, error) {
	p := &RecordingPlayer{filePath: filePath}
	if err := p.parse(); err != nil {
		return nil, err
	}
	return p, nil
}

// parse reads the entire .cast file into memory.
func (p *RecordingPlayer) parse() error {
	f, err := os.Open(p.filePath)
	if err != nil {
		return fmt.Errorf("failed to open recording %s: %w", p.filePath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase buffer size for large terminal output lines
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		lineNum++

		if lineNum == 1 {
			// First line is the header
			var header asciicastHeader
			if err := json.Unmarshal(line, &header); err != nil {
				return fmt.Errorf("failed to parse header: %w", err)
			}
			p.header = &header
			continue
		}

		// Subsequent lines are events: [time, type, data]
		var raw []json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue // skip malformed lines
		}
		if len(raw) < 3 {
			continue
		}

		var offset float64
		var eventType string
		var data string

		if err := json.Unmarshal(raw[0], &offset); err != nil {
			continue
		}
		if err := json.Unmarshal(raw[1], &eventType); err != nil {
			continue
		}
		if err := json.Unmarshal(raw[2], &data); err != nil {
			continue
		}

		p.events = append(p.events, RecordingEvent{
			Offset: offset,
			Type:   eventType,
			Data:   []byte(data),
		})
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading recording: %w", err)
	}

	return nil
}

// Header returns the recording header metadata.
func (p *RecordingPlayer) Header() *asciicastHeader {
	return p.header
}

// Events returns all parsed events.
func (p *RecordingPlayer) Events() []RecordingEvent {
	return p.events
}

// Duration returns the total duration of the recording in milliseconds.
func (p *RecordingPlayer) Duration() int64 {
	if len(p.events) == 0 {
		return 0
	}
	return int64(p.events[len(p.events)-1].Offset * 1000)
}

// StreamEvents streams events through a channel, respecting inter-event timing
// multiplied by the speed factor. Blocks until all events are sent or ctx is cancelled.
// speedMultiplier: 1.0 = real-time, 2.0 = 2x speed, 0.5 = half speed.
func (p *RecordingPlayer) StreamEvents(ctx context.Context, speedMultiplier float64) <-chan RecordingEvent {
	ch := make(chan RecordingEvent, 64)

	if speedMultiplier <= 0 {
		speedMultiplier = 1.0
	}

	go func() {
		defer close(ch)

		var lastOffset float64
		for i, event := range p.events {
			if i > 0 {
				// Wait for the inter-event delay, adjusted by speed
				delay := (event.Offset - lastOffset) / speedMultiplier
				if delay > 0 {
					// Cap max delay to 5 seconds (for long idle periods)
					if delay > 5.0 {
						delay = 5.0
					}
					timer := time.NewTimer(time.Duration(delay * float64(time.Second)))
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
				}
			}
			lastOffset = event.Offset

			select {
			case <-ctx.Done():
				return
			case ch <- event:
			}
		}
	}()

	return ch
}
