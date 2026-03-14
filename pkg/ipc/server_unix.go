//go:build unix || linux || darwin

package ipc

import (
	"fmt"
	"net"
	"os"
)

// createListener creates a Unix domain socket with restrictive permissions.
// The socket is created at the given path with 0600 permissions,
// ensuring only the owner can connect (EACCES for other users).
func createListener(path string) (net.Listener, error) {
	// Remove existing socket file if present
	if err := os.RemoveAll(path); err != nil {
		return nil, fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Create Unix domain socket
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on unix socket: %w", err)
	}

	// Set socket permissions to 0600 (owner read/write only)
	// This ensures EACCES for any other user attempting to connect
	if err := setSocketPermissions(path); err != nil {
		ln.Close()
		os.Remove(path)
		return nil, fmt.Errorf("failed to set socket permissions: %w", err)
	}

	return ln, nil
}

// setSocketPermissions sets the socket file permissions to 0600.
// This restricts access to the socket to only the owner,
// causing EACCES for any other user at the OS level.
func setSocketPermissions(path string) error {
	// Apply 0600 permissions (owner read/write only)
	// This is critical for the security requirement
	mode := os.FileMode(0600)

	if err := os.Chmod(path, mode); err != nil {
		return err
	}

	return nil
}

// cleanupSocket removes the Unix socket file.
// This is called when the server closes.
func cleanupSocket(path string) {
	if path != "" {
		os.Remove(path)
	}
}

// socketPathForSession returns the Unix socket path for a given session ID.
func socketPathForSession(sessionID string) string {
	return fmt.Sprintf("/tmp/daao-sess-%s.sock", sessionID)
}
