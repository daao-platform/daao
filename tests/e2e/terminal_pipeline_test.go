// Package e2e provides terminal pipeline end-to-end tests.
// This test exercises the full data flow: HTTP API → gRPC → WebSocket terminal stream.
// It requires a running Docker Compose stack (or NEXUS_TEST_URL / NEXUS_GRPC_ADDR env vars).
// Run with: go test -tags e2e -timeout 120s ./tests/e2e/... -run TestTerminalPipeline
package e2e

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectRoot walks up looking for go.mod to find the project root.
func projectRoot() string {
	dir, _ := os.Getwd()
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
	}
	return filepath.Join(dir, "..", "..")
}

// mockBinaryName returns the platform-correct binary name.
func mockBinaryName() string {
	if runtime.GOOS == "windows" {
		return "daao-mock.exe"
	}
	return "daao-mock"
}

// TestTerminalPipeline_E2E tests the full terminal data pipeline:
//  1. Create satellite via HTTP API
//  2. Start daao-mock as a subprocess connected via gRPC
//  3. Wait for satellite to become active
//  4. Create session via HTTP API
//  5. Connect WebSocket to /api/v1/sessions/{id}/stream
//  6. Verify mock output appears on WebSocket
//  7. Verify session terminates after TTL
//  8. Verify satellite goes offline after mock exits
//
// Requires: Docker Compose stack running OR NEXUS_TEST_URL + NEXUS_GRPC_ADDR set.
func TestTerminalPipeline_E2E(t *testing.T) {
	root := projectRoot()
	mockBinary := filepath.Join(root, "bin", mockBinaryName())

	// Build daao-mock if not present
	if _, err := os.Stat(mockBinary); os.IsNotExist(err) {
		cmd := exec.Command("go", "build", "-o", mockBinary, "./cmd/daao-mock")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "failed to build daao-mock: %s", out)
	}

	nexusURL := os.Getenv("NEXUS_TEST_URL")
	if nexusURL == "" {
		nexusURL = "https://localhost:8443"
	}
	grpcAddr := os.Getenv("NEXUS_GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = "localhost:8444"
	}

	// HTTP client that skips TLS verification (self-signed certs)
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	// Verify Nexus is reachable
	healthResp, err := client.Get(nexusURL + "/health")
	if err != nil {
		t.Skipf("Nexus not reachable at %s (start Docker Compose first): %v", nexusURL, err)
	}
	healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Skipf("Nexus /health returned %d", healthResp.StatusCode)
	}

	// ── Step 1: Create satellite ─────────────────────────────────
	t.Log("Step 1: Creating satellite via API")
	satBody := bytes.NewBufferString(`{"name":"e2e-pipeline-sat"}`)
	satResp, err := client.Post(nexusURL+"/api/v1/satellites", "application/json", satBody)
	require.NoError(t, err)
	defer satResp.Body.Close()
	require.Equal(t, http.StatusCreated, satResp.StatusCode, "satellite creation should return 201")

	var sat map[string]interface{}
	json.NewDecoder(satResp.Body).Decode(&sat)
	satID := sat["id"].(string)
	require.NotEmpty(t, satID)
	t.Logf("Created satellite: %s", satID)

	// ── Step 2: Start daao-mock subprocess ───────────────────────
	t.Log("Step 2: Starting daao-mock subprocess")
	expectedOutput := "Hello from E2E pipeline!\r\n"
	mockCmd := exec.Command(mockBinary,
		"--nexus-grpc", grpcAddr,
		"--name", "e2e-pipeline-sat",
		"--output", expectedOutput,
		"--session-ttl", "3s",
	)
	mockCmd.Stdout = os.Stdout
	mockCmd.Stderr = os.Stderr
	require.NoError(t, mockCmd.Start(), "failed to start daao-mock")
	defer func() {
		if mockCmd.Process != nil {
			mockCmd.Process.Kill()
			mockCmd.Wait()
		}
	}()

	// ── Step 3: Wait for satellite to become active ──────────────
	t.Log("Step 3: Waiting for satellite to become active")
	deadline := time.Now().Add(15 * time.Second)
	active := false
	for time.Now().Before(deadline) {
		listResp, err := client.Get(nexusURL + "/api/v1/satellites")
		if err == nil {
			var sats []map[string]interface{}
			json.NewDecoder(listResp.Body).Decode(&sats)
			listResp.Body.Close()

			for _, sm := range sats {
				if sm["id"] == satID && sm["status"] == "active" {
					active = true
					break
				}
			}
		}
		if active {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.True(t, active, "satellite should become active within 15s")
	t.Log("Satellite is active")

	// ── Step 4: Create session ───────────────────────────────────
	t.Log("Step 4: Creating session via API")
	sessBody := bytes.NewBufferString(fmt.Sprintf(`{"name":"e2e-session","satellite_id":"%s"}`, satID))
	sessResp, err := client.Post(nexusURL+"/api/v1/sessions", "application/json", sessBody)
	require.NoError(t, err)
	defer sessResp.Body.Close()
	require.Equal(t, http.StatusCreated, sessResp.StatusCode, "session creation should return 201")

	var sess map[string]interface{}
	json.NewDecoder(sessResp.Body).Decode(&sess)
	sessID := sess["id"].(string)
	require.NotEmpty(t, sessID)
	t.Logf("Created session: %s", sessID)

	// ── Step 5: Connect WebSocket to session stream ──────────────
	t.Log("Step 5: Connecting WebSocket to session stream")
	wsURL := "wss" + strings.TrimPrefix(nexusURL, "https") + "/api/v1/sessions/" + sessID + "/stream"
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err, "failed to connect WebSocket to %s", wsURL)
	defer ws.Close()

	// ── Step 6: Assert mock output appears on WS ─────────────────
	t.Log("Step 6: Checking for mock satellite output on WebSocket")
	receivedOutput := false
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := 0; i < 20; i++ {
		_, data, err := ws.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(data), "Hello from E2E pipeline!") {
			receivedOutput = true
			break
		}
	}
	assert.True(t, receivedOutput, "mock satellite output should appear on WebSocket")

	// ── Step 7: Assert terminated message when session ends ──────
	t.Log("Step 7: Waiting for session TTL to expire and terminal to close")
	terminatedReceived := false
	ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	for i := 0; i < 50; i++ {
		_, data, err := ws.ReadMessage()
		if err != nil {
			break
		}
		var msg map[string]string
		if json.Unmarshal(data, &msg) == nil && msg["type"] == "terminated" {
			terminatedReceived = true
			break
		}
	}
	assert.True(t, terminatedReceived, "should receive terminated message when session TTL expires")

	// ── Step 8: Kill mock and verify satellite goes offline ──────
	t.Log("Step 8: Killing mock satellite and verifying offline status")
	if mockCmd.Process != nil {
		mockCmd.Process.Kill()
		mockCmd.Wait()
	}

	deadline = time.Now().Add(10 * time.Second)
	offline := false
	for time.Now().Before(deadline) {
		listResp, err := client.Get(nexusURL + "/api/v1/satellites")
		if err == nil {
			var sats []map[string]interface{}
			json.NewDecoder(listResp.Body).Decode(&sats)
			listResp.Body.Close()
			for _, sm := range sats {
				if sm["id"] == satID && sm["status"] == "offline" {
					offline = true
					break
				}
			}
		}
		if offline {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	assert.True(t, offline, "satellite should go offline after mock exits")
}
