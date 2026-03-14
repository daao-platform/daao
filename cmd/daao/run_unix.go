//go:build !windows

// Package main provides Unix/Darwin stubs for Windows-specific run/daemon functions.
// The full daemon and PTY session management is currently Windows-specific;
// cross-compiled satellite binaries for Unix platforms will gain full
// support in a future release.
package main

import (
	"context"
	"fmt"
	"os"
)

// runCommand is not yet implemented for non-Windows platforms.
func runCommand(ctx context.Context, args []string) error {
	return fmt.Errorf("the 'run' command is not yet supported on this platform")
}

// getDaemonSocketPath returns the Unix domain socket path for the daemon control API.
func getDaemonSocketPath() string {
	homeDir, _ := os.UserHomeDir()
	return fmt.Sprintf("%s/.config/daao/daemon.sock", homeDir)
}
