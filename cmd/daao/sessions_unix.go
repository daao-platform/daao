//go:build !windows

// Package main provides Unix/Darwin stubs for Windows-specific session management commands.
// On non-Windows platforms, sessions are managed via Unix domain sockets by the daemon,
// and the CLI session/attach commands are not yet implemented.
package main

import (
	"context"
	"fmt"
)

// sessionsCommand is not yet implemented for non-Windows platforms.
func sessionsCommand(ctx context.Context, args []string) error {
	return fmt.Errorf("the 'sessions' command is not yet supported on this platform")
}

// attachCommand is not yet implemented for non-Windows platforms.
func attachCommand(ctx context.Context, args []string) error {
	return fmt.Errorf("the 'attach' command is not yet supported on this platform")
}
