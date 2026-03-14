// Package ipc provides inter-process communication via Unix Domain Sockets
// (POSIX) and Named Pipes (Windows).
package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
)

// DAAO-specific error codes (extending standard JSON-RPC 2.0 error codes from conn.go)
// Error codes: -32700 Parse error, -32600 Invalid Request, -32601 Method not found
// -32001 Auth error, -32002 Invalid state, -32003 Payload too large
const (
	AuthError       = -32001 // Authentication failed
	InvalidState    = -32002 // Invalid state value
	PayloadTooLarge = -32003 // Payload exceeds maximum size (1MB)
)

// Maximum OOB payload size (1MB)
const MaxOOBPayloadSize = 1024 * 1024

// Valid state values for daao.stateChange
var validStates = map[string]bool{
	"THINKING":       true,
	"INPUT_REQUIRED": true,
	"RUNNING":        true,
	"TOOL_CALLING":   true,
}

// Valid OOB UI component types for daao.oobUI
var validOOBComponents = map[string]bool{
	"FileDiff":        true,
	"MarkdownPreview": true,
	"FileTree":        true,
	"CommandResult":   true,
	"Progress":        true,
	"Confirmation":    true,
}

// StateChangeParams represents parameters for daao.stateChange method
type StateChangeParams struct {
	Token string `json:"token"`
	State string `json:"state"`
}

// OOBUIParams represents parameters for daao.oobUI method
type OOBUIParams struct {
	Token     string `json:"token"`
	Component string `json:"component"`
	Content   any    `json:"content"`
}

// Dispatcher handles JSON-RPC method dispatching with auth and validation
type Dispatcher struct {
	authToken string
}

// NewDispatcher creates a new JSON-RPC dispatcher
func NewDispatcher(authToken string) *Dispatcher {
	return &Dispatcher{
		authToken: authToken,
	}
}

// Dispatch processes a JSON-RPC message and returns a response
func (d *Dispatcher) Dispatch(msg *JSONRPCMessage) *JSONRPCResponse {
	// Check for valid JSON-RPC version
	if msg.JSONRPC != "2.0" {
		return NewErrorResponse(msg.ID, InvalidRequest, "Invalid Request: jsonrpc version must be 2.0")
	}

	// Method is required
	if msg.Method == "" {
		return NewErrorResponse(msg.ID, InvalidRequest, "Invalid Request: method is required")
	}

	// Route to the appropriate handler
	switch msg.Method {
	case "daao.stateChange":
		return d.handleStateChange(msg)
	case "daao.oobUI":
		return d.handleOOBUI(msg)
	default:
		return NewErrorResponse(msg.ID, MethodNotFound, fmt.Sprintf("Method not found: %s", msg.Method))
	}
}

// handleStateChange handles the daao.stateChange method
func (d *Dispatcher) handleStateChange(msg *JSONRPCMessage) *JSONRPCResponse {
	// Parse params
	var params StateChangeParams
	if msg.Params == nil {
		return NewErrorResponse(msg.ID, InvalidRequest, "Invalid Request: params are required")
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return NewErrorResponse(msg.ID, ParseError, fmt.Sprintf("Parse error: %v", err))
	}

	// Validate auth token
	if err := d.validateToken(params.Token); err != nil {
		return NewErrorResponse(msg.ID, AuthError, fmt.Sprintf("Auth error: %v", err))
	}

	// Validate state
	if params.State == "" {
		return NewErrorResponse(msg.ID, InvalidState, "Invalid state: state is required")
	}
	if !validStates[params.State] {
		return NewErrorResponse(msg.ID, InvalidState, fmt.Sprintf("Invalid state: %s (must be THINKING, INPUT_REQUIRED, RUNNING, or TOOL_CALLING)", params.State))
	}

	// Success
	result := map[string]string{
		"state":   params.State,
		"status": "ok",
	}
	resp, _ := NewResponse(msg.ID, result)
	return resp
}

// handleOOBUI handles the daao.oobUI method
func (d *Dispatcher) handleOOBUI(msg *JSONRPCMessage) *JSONRPCResponse {
	// Parse params
	var params OOBUIParams
	if msg.Params == nil {
		return NewErrorResponse(msg.ID, InvalidRequest, "Invalid Request: params are required")
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return NewErrorResponse(msg.ID, ParseError, fmt.Sprintf("Parse error: %v", err))
	}

	// Validate auth token
	if err := d.validateToken(params.Token); err != nil {
		return NewErrorResponse(msg.ID, AuthError, fmt.Sprintf("Auth error: %v", err))
	}

	// Validate component type
	if params.Component == "" {
		return NewErrorResponse(msg.ID, InvalidRequest, "Invalid Request: component type is required")
	}
	if !validOOBComponents[params.Component] {
		return NewErrorResponse(msg.ID, InvalidRequest, fmt.Sprintf("Invalid component type: %s (must be FileDiff, MarkdownPreview, FileTree, CommandResult, Progress, or Confirmation)", params.Component))
	}

	// Check payload size (1MB limit)
	if msg.Params != nil && len(msg.Params) > MaxOOBPayloadSize {
		return NewErrorResponse(msg.ID, PayloadTooLarge, fmt.Sprintf("Payload too large: maximum size is %d bytes", MaxOOBPayloadSize))
	}

	// Success
	result := map[string]string{
		"component": params.Component,
		"status":    "ok",
	}
	resp, _ := NewResponse(msg.ID, result)
	return resp
}

// validateToken validates the authentication token
func (d *Dispatcher) validateToken(token string) error {
	if token == "" {
		return errors.New("token is missing")
	}
	if d.authToken != "" && token != d.authToken {
		return errors.New("token is invalid")
	}
	return nil
}
