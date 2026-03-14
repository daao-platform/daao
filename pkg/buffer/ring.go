// Package buffer provides an ANSI-boundary-aware ring buffer for terminal scrollback.
package buffer

import (
	"bytes"
	"encoding/binary"
	"sync"
)

// DefaultCapacity is 5MB - the fixed capacity of the ring buffer.
const DefaultCapacity = 5 * 1024 * 1024

// RingBuffer is a thread-safe, fixed-capacity ring buffer that evicts only at
// ANSI escape sequence boundaries to preserve terminal fidelity.
type RingBuffer struct {
	mu       sync.Mutex
	data     []byte
	head     int // next write position
	length   int // current data length
	capacity int
}

// NewRingBuffer creates a new ring buffer with the specified capacity.
// If capacity is 0, DefaultCapacity (5MB) is used.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &RingBuffer{
		data:     make([]byte, capacity),
		head:     0,
		length:   0,
		capacity: capacity,
	}
}

// Write appends data to the ring buffer, evicting old data if necessary.
// It ensures eviction happens at ANSI sequence boundaries.
func (rb *RingBuffer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Write all data, evicting as needed
	for _, b := range p {
		rb.data[rb.head] = b
		rb.head = (rb.head + 1) % rb.capacity

		if rb.length < rb.capacity {
			rb.length++
		} else {
			// Buffer is full, need to evict
			// Find the correct position to evict (at ANSI boundary)
			evictPos := rb.findEvictPosition()
			rb.head = evictPos
		}
	}

	return len(p), nil
}

// findEvictPosition finds the position in the buffer where we can safely evict
// data without breaking an ANSI escape sequence.
// We need to find a position that is either:
// - At the start of the buffer
// - After a complete ANSI sequence
// - At a position that won't break a UTF-8 multi-byte sequence
func (rb *RingBuffer) findEvictPosition() int {
	// Current head position
	headPos := rb.head

	// Look back in the buffer to find the last safe eviction point
	// We need to scan backwards to find either:
	// 1. A position that starts a new valid sequence
	// 2. A position that's not in the middle of any sequence

	// Scan backwards up to 32 bytes (max ANSI sequence length)
	// to find a safe boundary
	searchLimit := 32
	if searchLimit > rb.length {
		searchLimit = rb.length
	}

	// Start from the position just before head
	searchStart := (headPos - 1 + rb.capacity) % rb.capacity

	// Search backwards for a safe boundary
	for i := 0; i < searchLimit; i++ {
		pos := (searchStart - i + rb.capacity) % rb.capacity
		prevPos := (pos - 1 + rb.capacity) % rb.capacity

		// Check if we can safely evict at this position
		// We need to ensure we're not breaking:
		// 1. A CSI sequence (ESC [)
		// 2. An OSC sequence (ESC ])
		// 3. A DCS sequence (ESC P)
		// 4. An SS2 sequence (ESC N)
		// 5. An SS3 sequence (ESC O)
		// 6. A UTF-8 multi-byte sequence

		// Check the byte at prevPos and pos to see if we're at a boundary
		if isANSIBoundary(rb.data, prevPos, rb.capacity, rb.length) {
			// This is a safe boundary, we can evict up to this point
			// (but not including this position)
			return pos
		}
	}

	// If we couldn't find a safe boundary, return the position that will
	// truncate the oldest data completely
	return (headPos - searchLimit + rb.capacity) % rb.capacity
}

// isANSIBoundary checks if the position in the buffer is at an ANSI sequence boundary.
// It checks if the byte at pos is the start of a new ANSI sequence or a regular character.
func isANSIBoundary(data []byte, pos int, capacity int, length int) bool {
	if length == 0 {
		return true
	}

	b := data[pos]

	// Check for ASCII control characters (0x00-0x1F)
	// These are always safe boundaries
	if b < 0x20 {
		return true
	}

	// Check for ESC (0x1B) - start of ANSI sequence
	if b == 0x1B || b == 0x9B {
		return true
	}

	// Check for UTF-8 continuation bytes (0x80-0xBF)
	// These should NOT be at the start of a sequence
	if b >= 0x80 && b <= 0xBF {
		return false
	}

	// Check for UTF-8 leading bytes (0xC0-0xFF)
	// These start multi-byte sequences
	if b >= 0xC0 {
		// This could be the start of a UTF-8 sequence
		// Check if it's a valid UTF-8 leading byte
		return true
	}

	// Regular ASCII character (0x20-0x7F)
	return true
}

// trimToANSIBoundary ensures the given length can be safely evicted without
// breaking any ANSI escape sequences. It returns the position in the buffer
// where eviction should stop (the first position that should be kept).
// The data parameter is the current buffer contents, and dataLen is the
// logical length of data in the buffer.
func trimToANSIBoundary(data []byte, dataLen int, capacity int) int {
	if dataLen == 0 {
		return 0
	}

	// The head position is at dataLen (the next write position)
	// We need to find a safe point before headPos to evict to
	// That safe point should be at an ANSI boundary

	// Find the head position (where new data would be written)
	headPos := dataLen % capacity

	// Search backwards from head to find a safe boundary
	searchLimit := 64 // Max length of any ANSI sequence
	if searchLimit > dataLen {
		searchLimit = dataLen
	}

	for i := 0; i < searchLimit; i++ {
		pos := (headPos - i - 1 + capacity) % capacity
		if isSafeEvictionPoint(data, pos, capacity, dataLen) {
			// Return the next position after this safe point
			return (pos + 1) % capacity
		}
	}

	// If no safe point found, return the oldest data position
	return (headPos - dataLen + capacity) % capacity
}

// isSafeEvictionPoint checks if it's safe to evict data up to (but not including) the given position.
// We need to ensure we're not cutting through an ANSI sequence.
func isSafeEvictionPoint(data []byte, pos int, capacity int, dataLen int) bool {
	if dataLen == 0 {
		return true
	}

	// Get the byte at the given position
	b := data[pos]

	// If we're at the start of the buffer, it's always safe
	if pos == 0 {
		return true
	}

	// Get the previous byte
	prevPos := (pos - 1 + capacity) % capacity
	prevB := data[prevPos]

	// Check for various ANSI sequence starts after the previous byte

	// Check for ESC followed by [ (CSI)
	if prevB == 0x1B && b == '[' {
		return true
	}

	// Check for ESC followed by ] (OSC)
	if prevB == 0x1B && b == ']' {
		return true
	}

	// Check for ESC followed by P (DCS)
	if prevB == 0x1B && b == 'P' {
		return true
	}

	// Check for ESC followed by N (SS2)
	if prevB == 0x1B && b == 'N' {
		return true
	}

	// Check for ESC followed by O (SS3)
	if prevB == 0x1B && b == 'O' {
		return true
	}

	// Check for ESC alone (incomplete sequence)
	if prevB == 0x1B {
		// Can't evict if previous was ESC (might be start of sequence)
		return false
	}

	// Check for UTF-8 continuation byte (can't start here)
	if b >= 0x80 && b <= 0xBF {
		return false
	}

	// Regular character or UTF-8 leading byte - safe to evict
	return true
}

// Snapshot returns a copy of the current buffer contents.
// This is used for buffer hydration when a client attaches.
// The snapshot is O(n) where n is the buffer length.
func (rb *RingBuffer) Snapshot() []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.length == 0 {
		return []byte{}
	}

	// Calculate the starting position (oldest data)
	start := (rb.head - rb.length + rb.capacity) % rb.capacity

	// Create a new slice with the correct length
	result := make([]byte, rb.length)

	if start+rb.length <= rb.capacity {
		// Data is contiguous
		copy(result, rb.data[start:start+rb.length])
	} else {
		// Data wraps around
		firstPart := rb.capacity - start
		copy(result, rb.data[start:])
		copy(result[firstPart:], rb.data[:rb.head])
	}

	return result
}

// Len returns the current number of bytes in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.length
}

// SnapshotWithLen returns a copy of the current buffer contents and the length
// atomically under a single lock acquisition. This avoids race conditions where
// data is written between separate Snapshot() and Len() calls.
func (rb *RingBuffer) SnapshotWithLen() ([]byte, int) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.length == 0 {
		return []byte{}, 0
	}

	start := (rb.head - rb.length + rb.capacity) % rb.capacity
	result := make([]byte, rb.length)

	if start+rb.length <= rb.capacity {
		copy(result, rb.data[start:start+rb.length])
	} else {
		firstPart := rb.capacity - start
		copy(result, rb.data[start:])
		copy(result[firstPart:], rb.data[:rb.head])
	}

	return result, rb.length
}

// Capacity returns the maximum capacity of the buffer.
func (rb *RingBuffer) Capacity() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.capacity
}

// Reset clears all data from the buffer.
func (rb *RingBuffer) Reset() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.head = 0
	rb.length = 0
}

// BinarySnapshot returns a binary snapshot of the ring buffer for serialization.
// Format: [length:4 bytes][capacity:4 bytes][data:length bytes]
func (rb *RingBuffer) BinarySnapshot() ([]byte, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	snapshot := make([]byte, 8+rb.length)
	binary.LittleEndian.PutUint32(snapshot[0:4], uint32(rb.length))
	binary.LittleEndian.PutUint32(snapshot[4:8], uint32(rb.capacity))

	if rb.length > 0 {
		start := (rb.head - rb.length + rb.capacity) % rb.capacity
		if start+rb.length <= rb.capacity {
			copy(snapshot[8:], rb.data[start:start+rb.length])
		} else {
			firstPart := rb.capacity - start
			copy(snapshot[8:], rb.data[start:])
			copy(snapshot[8+firstPart:], rb.data[:rb.head])
		}
	}

	return snapshot, nil
}

// ReadFromBinary restores a ring buffer from a binary snapshot.
func (rb *RingBuffer) ReadFromBinary(data []byte) error {
	if len(data) < 8 {
		return ErrInvalidSnapshot
	}

	length := binary.LittleEndian.Uint32(data[0:4])
	capacity := binary.LittleEndian.Uint32(data[4:8])

	if len(data) < 8+int(length) {
		return ErrInvalidSnapshot
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.capacity = int(capacity)
	rb.length = int(length)
	rb.head = int(length) % rb.capacity

	if len(rb.data) < rb.capacity {
		rb.data = make([]byte, rb.capacity)
	}

	if length > 0 {
		copy(rb.data, data[8:8+length])
	}

	return nil
}

// ErrInvalidSnapshot is returned when a snapshot is invalid.
var ErrInvalidSnapshot = bytes.ErrTooLarge

// Ensure ANSI sequence detection functions work correctly
// These are exported for testing purposes

// IsCSI checks if a sequence starting at pos is a CSI sequence (ESC [)
func IsCSI(data []byte, pos int) bool {
	if pos >= len(data)-1 {
		return false
	}
	return data[pos] == 0x1B && data[pos+1] == '['
}

// IsOSC checks if a sequence starting at pos is an OSC sequence (ESC ])
func IsOSC(data []byte, pos int) bool {
	if pos >= len(data)-1 {
		return false
	}
	return data[pos] == 0x1B && data[pos+1] == ']'
}

// IsDCS checks if a sequence starting at pos is a DCS sequence (ESC P)
func IsDCS(data []byte, pos int) bool {
	if pos >= len(data)-1 {
		return false
	}
	return data[pos] == 0x1B && data[pos+1] == 'P'
}

// IsSS2 checks if a sequence starting at pos is an SS2 sequence (ESC N)
func IsSS2(data []byte, pos int) bool {
	if pos >= len(data)-1 {
		return false
	}
	return data[pos] == 0x1B && data[pos+1] == 'N'
}

// IsSS3 checks if a sequence starting at pos is an SS3 sequence (ESC O)
func IsSS3(data []byte, pos int) bool {
	if pos >= len(data)-1 {
		return false
	}
	return data[pos] == 0x1B && data[pos+1] == 'O'
}

// IsUTF8MultibyteStart checks if a byte is the start of a UTF-8 multi-byte sequence
func IsUTF8MultibyteStart(b byte) bool {
	return b >= 0xC0
}

// IsUTF8Continuation checks if a byte is a UTF-8 continuation byte
func IsUTF8Continuation(b byte) bool {
	return b >= 0x80 && b <= 0xBF
}
