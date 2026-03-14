//go:build unix || linux || darwin

package main

import (
	"net"
	"os"
)

// controlListen creates a Unix domain socket listener for the control API.
func controlListen(path string) (net.Listener, error) {
	// Remove stale socket
	os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// Restrict to owner
	os.Chmod(path, 0600)
	return ln, nil
}

// controlDial connects to the daemon control socket.
func controlDial(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}

// controlCleanup removes the socket file.
func controlCleanup(path string) {
	os.Remove(path)
}
