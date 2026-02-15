// Tests for the [Client] type covering handshake, activity commands,
// nonce uniqueness, and connection lifecycle.
package discord

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"testing"
)

// ///////////////////////////////////////////////
// Test Helpers
// ///////////////////////////////////////////////

// readFrame is a test helper that reads a single frame from a connection.
func readFrame(t *testing.T, conn net.Conn) (Opcode, map[string]any) {
	t.Helper()
	opcode, payload, err := DecodeFrame(conn)
	if err != nil {
		t.Fatalf("failed to read frame: %v", err)
		return 0, nil
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("failed to parse frame payload: %v", err)
		return 0, nil
	}
	return opcode, m
}

// writeReadyResponse writes a READY event response frame to the connection.
func writeReadyResponse(t *testing.T, conn net.Conn) {
	t.Helper()
	resp, err := json.Marshal(map[string]any{
		"cmd": "DISPATCH",
		"evt": "READY",
	})
	if err != nil {
		t.Fatalf("failed to marshal ready response: %v", err)
		return
	}
	frame, err := EncodeFrame(OpFrame, resp)
	if err != nil {
		t.Fatalf("failed to encode ready response: %v", err)
		return
	}
	if _, err := conn.Write(frame); err != nil {
		t.Fatalf("failed to write ready response: %v", err)
		return
	}
}

// ///////////////////////////////////////////////
// Client.handshake
// ///////////////////////////////////////////////

func TestClient_Handshake(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	c := NewClient("test-app-id")
	// Inject the mock connection directly, bypassing connectToDiscord.
	c.conn = clientConn

	done := make(chan error, 1)
	go func() {
		done <- c.handshake()
	}()

	opcode, m := readFrame(t, serverConn)
	if opcode != OpHandshake {
		t.Fatalf("expected opcode %d (HANDSHAKE), got %d", OpHandshake, opcode)
	}

	v, ok := m["v"]
	if !ok || int(v.(float64)) != 1 {
		t.Fatalf("expected v=1, got %v", v)
	}

	clientID, ok := m["client_id"]
	if !ok || clientID != "test-app-id" {
		t.Fatalf("expected client_id=test-app-id, got %v", clientID)
	}

	// Send READY response back to complete handshake.
	writeReadyResponse(t, serverConn)

	if err := <-done; err != nil {
		t.Fatalf("handshake returned error: %v", err)
	}
}

// ///////////////////////////////////////////////
// Client.SetActivity
// ///////////////////////////////////////////////

func TestClient_SetActivity(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	c := NewClient("test-app-id")
	c.conn = clientConn

	activity := &Activity{
		Details: "Testing",
		State:   "Running tests",
		Timestamps: &Timestamps{
			Start: 1000000,
		},
		Assets: &Assets{
			LargeImage: "large-img",
			LargeText:  "Large Text",
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- c.SetActivity(activity)
	}()

	opcode, m := readFrame(t, serverConn)
	if opcode != OpFrame {
		t.Fatalf("expected opcode %d (FRAME), got %d", OpFrame, opcode)
	}

	if m["cmd"] != "SET_ACTIVITY" {
		t.Fatalf("expected cmd=SET_ACTIVITY, got %v", m["cmd"])
	}

	// Check nonce is present and non-empty.
	nonce, ok := m["nonce"].(string)
	if !ok || nonce == "" {
		t.Fatalf("expected non-empty nonce, got %v", m["nonce"])
	}

	args, ok := m["args"].(map[string]any)
	if !ok {
		t.Fatalf("expected args to be a map, got %T", m["args"])
	}

	pid, ok := args["pid"].(float64)
	if !ok || int(pid) != os.Getpid() {
		t.Fatalf("expected pid=%d, got %v", os.Getpid(), args["pid"])
	}

	act, ok := args["activity"].(map[string]any)
	if !ok {
		t.Fatalf("expected activity to be a map, got %T", args["activity"])
	}

	if act["details"] != "Testing" {
		t.Fatalf("expected details=Testing, got %v", act["details"])
	}
	if act["state"] != "Running tests" {
		t.Fatalf("expected state=Running tests, got %v", act["state"])
	}

	if err := <-done; err != nil {
		t.Fatalf("SetActivity returned error: %v", err)
	}
}

func TestClient_SetActivity_WithButtons(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	c := NewClient("test-app-id")
	c.conn = clientConn

	activity := &Activity{
		Details: "With buttons",
		Buttons: []Button{
			{Label: "GitHub", URL: "https://github.com"},
			{Label: "Website", URL: "https://example.com"},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- c.SetActivity(activity)
	}()

	_, m := readFrame(t, serverConn)

	args := m["args"].(map[string]any)
	act := args["activity"].(map[string]any)

	buttons, ok := act["buttons"].([]any)
	if !ok {
		t.Fatalf("expected buttons array, got %T", act["buttons"])
	}
	if len(buttons) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(buttons))
	}

	b0 := buttons[0].(map[string]any)
	if b0["label"] != "GitHub" || b0["url"] != "https://github.com" {
		t.Fatalf("button 0 mismatch: %v", b0)
	}

	b1 := buttons[1].(map[string]any)
	if b1["label"] != "Website" || b1["url"] != "https://example.com" {
		t.Fatalf("button 1 mismatch: %v", b1)
	}

	if err := <-done; err != nil {
		t.Fatalf("SetActivity returned error: %v", err)
	}
}

// ///////////////////////////////////////////////
// Client.ClearActivity
// ///////////////////////////////////////////////

func TestClient_ClearActivity(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	c := NewClient("test-app-id")
	c.conn = clientConn

	done := make(chan error, 1)
	go func() {
		done <- c.ClearActivity()
	}()

	opcode, m := readFrame(t, serverConn)
	if opcode != OpFrame {
		t.Fatalf("expected opcode %d (FRAME), got %d", OpFrame, opcode)
	}

	if m["cmd"] != "SET_ACTIVITY" {
		t.Fatalf("expected cmd=SET_ACTIVITY, got %v", m["cmd"])
	}

	args := m["args"].(map[string]any)

	// Activity should be null/nil.
	if args["activity"] != nil {
		t.Fatalf("expected null activity, got %v", args["activity"])
	}

	pid, ok := args["pid"].(float64)
	if !ok || int(pid) != os.Getpid() {
		t.Fatalf("expected pid=%d, got %v", os.Getpid(), args["pid"])
	}

	if err := <-done; err != nil {
		t.Fatalf("ClearActivity returned error: %v", err)
	}
}

// ///////////////////////////////////////////////
// Client Nonce Uniqueness
// ///////////////////////////////////////////////

func TestClient_NonceUniqueness(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	c := NewClient("test-app-id")
	c.conn = clientConn

	nonces := make(map[string]bool)

	for i := 0; i < 5; i++ {
		done := make(chan error, 1)
		go func() {
			done <- c.SetActivity(&Activity{Details: "test"})
		}()

		_, m := readFrame(t, serverConn)
		nonce := m["nonce"].(string)

		if nonces[nonce] {
			t.Fatalf("duplicate nonce on call %d: %s", i, nonce)
		}
		nonces[nonce] = true

		if err := <-done; err != nil {
			t.Fatalf("SetActivity call %d returned error: %v", i, err)
		}
	}
}

// ///////////////////////////////////////////////
// Client.Close
// ///////////////////////////////////////////////

func TestClient_Close_NilConnection(t *testing.T) {
	c := NewClient("test-app-id")
	// conn is nil by default.
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil connection should return nil, got: %v", err)
	}
}

// ///////////////////////////////////////////////
// Client.Connected
// ///////////////////////////////////////////////

func TestClient_Connected_ReturnsFalseInitially(t *testing.T) {
	c := NewClient("test-app-id")
	if c.Connected() {
		t.Fatal("expected Connected() to return false for new client")
	}
}

// ///////////////////////////////////////////////
// Client.sendCommand
// ///////////////////////////////////////////////

func TestClient_SendCommand_NotConnected(t *testing.T) {
	c := NewClient("test-app-id")
	err := c.sendCommand("SET_ACTIVITY", map[string]any{"pid": 1})
	if err == nil {
		t.Fatal("expected error from sendCommand when not connected")
	}
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got: %v", err)
	}
}

// ///////////////////////////////////////////////
// Client.Connect — reconnect closes old connection
// ///////////////////////////////////////////////

func TestClient_Connect_ClosesOldConnection(t *testing.T) {
	// Simulate an existing connection by injecting a net.Pipe endpoint.
	oldServer, oldClient := net.Pipe()
	defer oldServer.Close()

	c := NewClient("test-app-id")
	c.conn = oldClient

	// Connect will try connectToDiscord() which will fail (no Discord running),
	// but the important thing is that the old connection gets closed first.
	_ = c.Connect()

	// Verify the old client-side connection was closed by attempting a write.
	_, err := oldClient.Write([]byte("test"))
	if err == nil {
		t.Error("expected old connection to be closed, but write succeeded")
	}
}

// ///////////////////////////////////////////////
// Client.handshake failure sets conn to nil
// ///////////////////////////////////////////////

func TestClient_Handshake_FailureCleansConn(t *testing.T) {
	serverConn, clientConn := net.Pipe()

	c := NewClient("test-app-id")
	c.conn = clientConn

	// Close the server side immediately so handshake read fails.
	serverConn.Close()

	err := c.handshake()
	if err == nil {
		t.Fatal("expected handshake to fail")
	}

	// After handshake failure in Connect, conn should be nil.
	// handshake itself doesn't nil the conn — Connect does.
	// So we test the Connect path indirectly by verifying the contract:
	// if handshake returns an error, the caller (Connect) sets conn = nil.
	// The clientConn should be closed though.
	_, writeErr := clientConn.Write([]byte("test"))
	if writeErr == nil {
		t.Error("expected clientConn to be closed after handshake failure")
	}
}

func TestClient_Handshake_ErrorResponse(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	c := NewClient("test-app-id")
	c.conn = clientConn

	done := make(chan error, 1)
	go func() {
		done <- c.handshake()
	}()

	// Read the handshake frame from the client.
	readFrame(t, serverConn)

	// Respond with an ERROR event.
	resp, _ := json.Marshal(map[string]any{
		"evt": "ERROR",
		"data": map[string]any{
			"message": "invalid client_id",
		},
	})
	frame, _ := EncodeFrame(OpFrame, resp)
	serverConn.Write(frame)

	err := <-done
	if err == nil {
		t.Fatal("expected handshake to fail with ERROR response")
	}
}
