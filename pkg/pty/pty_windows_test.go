//go:build windows

package pty

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestConPTYReadWrite(t *testing.T) {
	p, err := NewPty(80, 24)
	if err != nil {
		t.Fatalf("Failed to create ConPTY: %v", err)
	}
	defer p.Close()

	// Spawn cmd.exe /c echo hello — a short-lived process that writes to the ConPTY
	proc, err := p.Start(
		`C:\Windows\System32\cmd.exe`,
		[]string{`C:\Windows\System32\cmd.exe`, "/c", "echo", "hello"},
		"",   // inherit working dir
		nil,  // inherit env
		0,    // no detach flags
	)
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}
	defer proc.Kill()

	// Read output — cmd.exe /c echo should produce "hello\r\n" on the ConPTY pipe
	buf := make([]byte, 4096)
	var output []byte
	deadline := time.Now().Add(5 * time.Second)

	for {
		if time.Now().After(deadline) {
			t.Fatalf("Timed out waiting for output. Got so far: %q", string(output))
		}

		p.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, readErr := p.Read(buf)
		if n > 0 {
			output = append(output, buf[:n]...)
		}

		if bytes.Contains(output, []byte("hello")) {
			break
		}

		if readErr != nil {
			if os.IsTimeout(readErr) {
				continue
			}
			// Process may have exited — check what we got
			if len(output) > 0 && bytes.Contains(output, []byte("hello")) {
				break
			}
			t.Fatalf("Read error: %v (output so far: %q)", readErr, string(output))
		}
	}

	t.Logf("ConPTY output: %q", string(output))
}

func TestConPTYResize(t *testing.T) {
	p, err := NewPty(80, 24)
	if err != nil {
		t.Fatalf("Failed to create ConPTY: %v", err)
	}
	defer p.Close()

	// Resize should not error
	if err := p.Resize(120, 40); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}
}

func TestConPTYCloseIdempotent(t *testing.T) {
	p, err := NewPty(80, 24)
	if err != nil {
		t.Fatalf("Failed to create ConPTY: %v", err)
	}

	// Close should be safe to call multiple times (sync.Once)
	if err := p.Close(); err != nil {
		t.Fatalf("First Close failed: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Second Close should not fail: %v", err)
	}
}

func TestConPTYReadAfterClose(t *testing.T) {
	p, err := NewPty(80, 24)
	if err != nil {
		t.Fatalf("Failed to create ConPTY: %v", err)
	}

	p.Close()

	buf := make([]byte, 128)
	_, err = p.Read(buf)
	if err == nil {
		t.Error("Expected error reading from closed ConPTY, got nil")
	}
}
