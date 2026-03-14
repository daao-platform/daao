//go:build windows

package ipc

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// createListener creates a Windows Named Pipe with security restrictions.
// The pipe is restricted to the current user SID to prevent access by other users.
func createListener(path string) (net.Listener, error) {
	// On Windows, path is the pipe name
	// Convert /tmp/daao-sess-<uuid>.sock format to Windows pipe name
	pipeName := convertToPipeName(path)

	// Create security descriptor restricting access to current user
	sa, err := createSecurityAttributes()
	if err != nil {
		return nil, fmt.Errorf("failed to create security attributes: %w", err)
	}

	// Convert pipe name to Windows UTF16
	namePtr, err := windows.UTF16PtrFromString(pipeName)
	if err != nil {
		return nil, fmt.Errorf("failed to convert pipe name: %w", err)
	}

	// Create the first instance of the pipe
	handle, err := windows.CreateNamedPipe(
		namePtr,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_MESSAGE|windows.PIPE_READMODE_MESSAGE|windows.PIPE_WAIT,
		1, // Max instances
		0, // Out buffer size
		0, // In buffer size
		0, // Timeout
		sa,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create named pipe: %w", err)
	}

	ln := &windowsPipeListener{
		handle:   handle,
		pipeName: pipeName,
		addr:     &windowsPipeAddr{path: pipeName},
	}

	return ln, nil
}

// convertToPipeName converts a Unix-style path to a Windows pipe name.
func convertToPipeName(path string) string {
	name := path
	if len(name) > 4 && name[len(name)-4:] == ".sock" {
		name = name[:len(name)-4]
	}
	if len(name) > 4 && name[:4] == "/tmp" {
		name = name[4:]
	}
	for len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}
	return "\\\\.\\pipe\\" + name
}

// createSecurityAttributes creates a SECURITY_ATTRIBUTES structure
// that restricts access to the current user.
func createSecurityAttributes() (*windows.SecurityAttributes, error) {
	// Get current process token
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return nil, fmt.Errorf("failed to open process token: %w", err)
	}
	defer token.Close()

	// Get user SID from token
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("failed to get token user: %w", err)
	}

	// Create a security descriptor
	sd, err := windows.NewSecurityDescriptor()
	if err != nil {
		return nil, err
	}

	// Set the owner to current user
	if err := sd.SetOwner(user.User.Sid, false); err != nil {
		return nil, fmt.Errorf("failed to set owner: %w", err)
	}

	// Set the primary group to current user
	if err := sd.SetGroup(user.User.Sid, false); err != nil {
		return nil, fmt.Errorf("failed to set group: %w", err)
	}

	// Convert to SecurityAttributes
	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
		InheritHandle:      0,
	}

	return sa, nil
}

// windowsPipeListener implements net.Listener for Windows Named Pipes.
type windowsPipeListener struct {
	handle   windows.Handle
	pipeName string
	addr     *windowsPipeAddr
	closed   bool
}

// Accept waits for and returns the next connection to the listener.
func (l *windowsPipeListener) Accept() (net.Conn, error) {
	if l.closed {
		return nil, &net.OpError{Op: "accept", Err: os.ErrClosed}
	}

	// Wait for client to connect
	err := windows.ConnectNamedPipe(l.handle, nil)
	if err != nil && err != windows.ERROR_PIPE_CONNECTED {
		return nil, fmt.Errorf("failed to connect named pipe: %w", err)
	}

	conn := &windowsPipeConn{
		handle: l.handle,
		addr:   l.addr,
	}

	// Create a new handle for the next connection
	sa, err := createSecurityAttributes()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create security attributes for next pipe: %w", err)
	}

	namePtr, err := windows.UTF16PtrFromString(l.pipeName)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to convert pipe name: %w", err)
	}

	newHandle, err := windows.CreateNamedPipe(
		namePtr,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_MESSAGE|windows.PIPE_READMODE_MESSAGE|windows.PIPE_WAIT,
		1,
		0,
		0,
		0,
		sa,
	)
	if err != nil {
		l.closed = true
	} else {
		l.handle = newHandle
	}

	return conn, nil
}

// Close closes the listener.
func (l *windowsPipeListener) Close() error {
	l.closed = true
	if l.handle != windows.InvalidHandle {
		return windows.CloseHandle(l.handle)
	}
	return nil
}

// Addr returns the listener's network address.
func (l *windowsPipeListener) Addr() net.Addr {
	return l.addr
}

// windowsPipeAddr represents the address of a Windows Named Pipe.
type windowsPipeAddr struct {
	path string
}

func (a *windowsPipeAddr) Network() string {
	return "pipe"
}

func (a *windowsPipeAddr) String() string {
	return a.path
}

// windowsPipeConn implements net.Conn for Windows Named Pipes.
type windowsPipeConn struct {
	handle windows.Handle
	addr   net.Addr
	closed bool
}

// Read reads data from the connection.
func (c *windowsPipeConn) Read(b []byte) (n int, err error) {
	if c.closed {
		return 0, os.ErrClosed
	}

	var bytesRead uint32
	err = windows.ReadFile(c.handle, b, &bytesRead, nil)
	if err != nil {
		if err == windows.ERROR_BROKEN_PIPE {
			return 0, io.EOF
		}
		return int(bytesRead), err
	}
	return int(bytesRead), nil
}

// Write writes data to the connection.
func (c *windowsPipeConn) Write(b []byte) (n int, err error) {
	if c.closed {
		return 0, os.ErrClosed
	}

	var bytesWritten uint32
	err = windows.WriteFile(c.handle, b, &bytesWritten, nil)
	if err != nil {
		return int(bytesWritten), err
	}
	return int(bytesWritten), nil
}

// Close closes the connection.
func (c *windowsPipeConn) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return windows.CloseHandle(c.handle)
}

// LocalAddr returns the local network address.
func (c *windowsPipeConn) LocalAddr() net.Addr {
	return c.addr
}

// RemoteAddr returns the remote network address.
func (c *windowsPipeConn) RemoteAddr() net.Addr {
	return c.addr
}

// SetDeadline sets the read and write deadlines.
func (c *windowsPipeConn) SetDeadline(t time.Time) error {
	_ = t
	return nil
}

// SetReadDeadline sets the read deadline.
func (c *windowsPipeConn) SetReadDeadline(t time.Time) error {
	_ = t
	return nil
}

// SetWriteDeadline sets the write deadline.
func (c *windowsPipeConn) SetWriteDeadline(t time.Time) error {
	_ = t
	return nil
}

// cleanupSocket is a no-op on Windows (Named Pipes are cleaned up automatically).
func cleanupSocket(path string) {
	_ = path
}

// socketPathForSession returns the Windows pipe name for a given session ID.
func socketPathForSession(sessionID string) string {
	return fmt.Sprintf("/tmp/daao-sess-%s.sock", sessionID)
}
