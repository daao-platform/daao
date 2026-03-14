// Package ipc provides inter-process communication via Unix Domain Sockets
// (POSIX) and Named Pipes (Windows).
package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"
)

// Conn represents an IPC connection to a client.
type Conn struct {
	conn    net.Conn
	encoder *json.Encoder
	mu      sync.Mutex
	err     error // connection error
}

// newConn creates a new IPC connection.
func newConn(nc net.Conn, _ *Server) *Conn {
	return &Conn{
		conn:    nc,
		encoder: json.NewEncoder(nc),
	}
}

// Read implements io.Reader - reads raw bytes from the connection.
func (c *Conn) Read(p []byte) (n int, err error) {
	return c.conn.Read(p)
}

// ReadJSON reads a JSON-RPC message from the connection.
func (c *Conn) ReadJSON(msg *JSONRPCMessage) error {
	decoder := json.NewDecoder(c.conn)
	return decoder.Decode(msg)
}

// SendResponse sends a JSON-RPC response.
func (c *Conn) SendResponse(resp *JSONRPCResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.encoder.Encode(resp)
}

// SendRequest sends a JSON-RPC request.
func (c *Conn) SendRequest(req *JSONRPCRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.encoder.Encode(req)
}

// SendNotification sends a JSON-RPC notification (request without ID).
func (c *Conn) SendNotification(method string, params interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	var paramsJSON json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		paramsJSON = b
	}
	
	notif := JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsJSON,
	}
	return c.encoder.Encode(notif)
}

// SendMessage sends a raw JSON-RPC message.
func (c *Conn) SendMessage(msg *JSONRPCMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.encoder.Encode(msg)
}

// SetReadDeadline sets the read deadline.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// SetDeadline sets both read and write deadlines.
func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// Close closes the connection.
func (c *Conn) Close() error {
	return c.conn.Close()
}

// LocalAddr returns the local network address.
func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// Err returns the connection error, if any.
func (c *Conn) Err() error {
	return c.err
}

// JSONRPCMessage represents a JSON-RPC 2.0 message.
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}    `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}    `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      interface{}    `json:"id"`
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// NewRequest creates a new JSON-RPC request.
func NewRequest(id interface{}, method string, params interface{}) (*JSONRPCRequest, error) {
	var paramsJSON json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		paramsJSON = b
	}
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsJSON,
		ID:      id,
	}, nil
}

// NewResponse creates a new JSON-RPC response with result.
func NewResponse(id interface{}, result interface{}) (*JSONRPCResponse, error) {
	var resultJSON json.RawMessage
	if result != nil {
		b, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		resultJSON = b
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  resultJSON,
		ID:      id,
	}, nil
}

// NewErrorResponse creates a new JSON-RPC error response.
func NewErrorResponse(id interface{}, code int, message string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
		ID: id,
	}
}

// ParseErrorCode returns standard JSON-RPC error codes.
const (
	ParseError     = -32700     // Invalid JSON was received
	InvalidRequest = -32600     // The JSON sent is not a valid Request object
	MethodNotFound = -32601     // The method does not exist
	InvalidParams  = -32602     // Invalid method parameter(s)
	InternalError  = -32603     // Internal JSON-RPC error
	
	// Application-defined errors
	ServerError = -32000 // Reserved for implementation-defined server-errors
)

// Client represents an IPC client that can connect to a server.
type Client struct {
	conn *Conn
}

// Dial connects to an IPC server at the given socket path.
func Dial(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}
	
	return &Client{
		conn: newConn(conn, nil),
	}, nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Call sends a JSON-RPC request and waits for a response.
func (c *Client) Call(ctx context.Context, method string, params interface{}, result interface{}) error {
	req, err := NewRequest(uint64(1), method, params)
	if err != nil {
		return err
	}
	
	if err := c.conn.SendRequest(req); err != nil {
		return err
	}
	
	// Set deadline for response
	deadline, ok := ctx.Deadline()
	if ok {
		c.conn.SetReadDeadline(deadline)
	}
	
	var msg JSONRPCMessage
	if err := c.conn.ReadJSON(&msg); err != nil {
		return err
	}
	
	if msg.Error != nil {
		return errors.New(msg.Error.Message)
	}
	
	if msg.Result != nil && result != nil {
		return json.Unmarshal(msg.Result, result)
	}
	
	return nil
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *Client) Notify(ctx context.Context, method string, params interface{}) error {
	return c.conn.SendNotification(method, params)
}

// Conn returns the underlying connection.
func (c *Client) Conn() *Conn {
	return c.conn
}
