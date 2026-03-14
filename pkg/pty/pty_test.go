//go:build !windows

package pty

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestPtyReadWrite(t *testing.T) {
	p, err := NewPty(80, 24)
	if err != nil {
		t.Skipf("Skipping PTY test: failed to create PTY: %v", err)
	}
	defer p.Close()

	// Use 'cat' to echo input back
	proc, err := p.Start("cat", []string{"cat"}, "", nil, 0)
	if err != nil {
		t.Fatalf("Failed to start 'cat': %v", err)
	}
	defer proc.Kill()

	input := []byte("hello pty\n")
	n, err := p.Write(input)
	if err != nil {
		t.Fatalf("Failed to write to PTY: %v", err)
	}
	if n != len(input) {
		t.Fatalf("Wrote %d bytes, expected %d", n, len(input))
	}

	// Read back the echoed output with a timeout
	buf := make([]byte, 1024)
	var output []byte
	deadline := time.Now().Add(2 * time.Second)

	for {
		if time.Now().After(deadline) {
			t.Fatalf("Timed out waiting for echoed output. Got: %q", string(output))
		}

		err = p.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		if err != nil {
			t.Fatalf("Failed to set read deadline: %v", err)
		}

		n, err = p.Read(buf)
		if n > 0 {
			output = append(output, buf[:n]...)
		}

		if bytes.Contains(output, []byte("hello pty")) {
			break
		}

		if err != nil {
			if os.IsTimeout(err) {
				continue
			}
			t.Fatalf("Failed to read from PTY: %v", err)
		}
	}
}

func TestPtyReadAfterClose(t *testing.T) {
	p, err := NewPty(80, 24)
	if err != nil {
		t.Skipf("Skipping PTY test: failed to create PTY: %v", err)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Failed to close PTY: %v", err)
	}

	buf := make([]byte, 128)
	_, err = p.Read(buf)
	if err == nil {
		t.Error("Expected error when reading from closed PTY, got nil")
	}
}

func TestPtySetReadDeadline(t *testing.T) {
	p, err := NewPty(80, 24)
	if err != nil {
		t.Skipf("Skipping PTY test: failed to create PTY: %v", err)
	}
	defer p.Close()

	// Set a very short deadline
	err = p.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if err != nil {
		t.Fatalf("Failed to set read deadline: %v", err)
	}

	buf := make([]byte, 128)
	_, err = p.Read(buf)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	} else if !os.IsTimeout(err) {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}
