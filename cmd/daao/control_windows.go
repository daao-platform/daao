//go:build windows

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// controlListen creates a TCP listener on localhost for the control API.
// On Windows we use TCP on an ephemeral port and write the port to a known file
// since Windows Named Pipes require go-winio which isn't a project dependency.
func controlListen(path string) (net.Listener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	// Write the port to a known file so the CLI can discover it.
	port := ln.Addr().(*net.TCPAddr).Port
	portFile := controlPortFile(path)
	os.MkdirAll(filepath.Dir(portFile), 0700)
	if err := os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("failed to write control port file: %w", err)
	}

	return ln, nil
}

// controlDial connects to the daemon control socket on Windows.
func controlDial(path string) (net.Conn, error) {
	portFile := controlPortFile(path)
	data, err := os.ReadFile(portFile)
	if err != nil {
		return nil, fmt.Errorf("daemon not running (no control port file): %w", err)
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid control port file: %w", err)
	}
	return net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
}

// controlCleanup removes the port file.
func controlCleanup(path string) {
	os.Remove(controlPortFile(path))
}

// controlPortFile returns the path to the file that stores the control port.
func controlPortFile(path string) string {
	return path + ".port"
}
