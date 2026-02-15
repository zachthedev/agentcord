// Package discord provides a client for Discord's local IPC socket,
// enabling Rich Presence updates via the SET_ACTIVITY command.
//
// The [Client] type manages connection lifecycle and command framing.
// Platform-specific socket discovery is handled by conn_unix.go and
// conn_windows.go.
package discord

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
)

// ///////////////////////////////////////////////
// Sentinel Errors
// ///////////////////////////////////////////////

// ErrNotConnected is returned when an operation requires an active connection.
var ErrNotConnected = errors.New("not connected")

// ///////////////////////////////////////////////
// Data Types
// ///////////////////////////////////////////////

// Button represents a clickable button in a Discord Rich Presence activity.
type Button struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Timestamps holds the start timestamp for an activity.
type Timestamps struct {
	Start int64 `json:"start,omitempty"`
}

// Assets holds image keys and tooltip text for an activity.
type Assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

// Activity represents a Discord Rich Presence activity.
type Activity struct {
	Details    string      `json:"details,omitempty"`
	State      string      `json:"state,omitempty"`
	Timestamps *Timestamps `json:"timestamps,omitempty"`
	Assets     *Assets     `json:"assets,omitempty"`
	Buttons    []Button    `json:"buttons,omitempty"`
}

// ///////////////////////////////////////////////
// Client
// ///////////////////////////////////////////////

// Client manages a connection to Discord's IPC socket.
type Client struct {
	// appID is the Discord application (OAuth2 client) identifier.
	appID string

	// mu protects conn and nonce from concurrent access.
	mu sync.Mutex
	// conn is the active IPC socket connection, or nil when disconnected.
	conn net.Conn
	// nonce is a monotonically increasing counter used to tag each command frame.
	nonce uint64
}

// NewClient creates a new Discord IPC client for the given application ID.
func NewClient(appID string) *Client {
	return &Client{appID: appID}
}

// Connect establishes a connection to Discord via IPC and sends the handshake.
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Close old connection if reconnecting.
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	conn, err := connectToDiscord()
	if err != nil {
		return err
	}
	c.conn = conn

	if err := c.handshake(); err != nil {
		c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// SetActivity sends a SET_ACTIVITY command to Discord.
func (c *Client) SetActivity(activity *Activity) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.sendCommand("SET_ACTIVITY", map[string]any{
		"pid":      os.Getpid(),
		"activity": activity,
	})
}

// ClearActivity sends a SET_ACTIVITY command with a nil activity.
func (c *Client) ClearActivity() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.sendCommand("SET_ACTIVITY", map[string]any{
		"pid":      os.Getpid(),
		"activity": nil,
	})
}

// Close clears the activity and closes the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	// Best-effort clear before closing.
	_ = c.sendCommand("SET_ACTIVITY", map[string]any{
		"pid":      os.Getpid(),
		"activity": nil,
	})

	err := c.conn.Close()
	c.conn = nil
	return err
}

// Connected reports whether the client has an active connection.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}

// handshake sends the initial handshake frame to Discord and validates the
// response. The caller must hold c.mu.
func (c *Client) handshake() error {
	payload, err := json.Marshal(map[string]any{
		"v":         1,
		"client_id": c.appID,
	})
	if err != nil {
		return fmt.Errorf("marshaling handshake: %w", err)
	}

	frame, err := EncodeFrame(OpHandshake, payload)
	if err != nil {
		return fmt.Errorf("encoding handshake: %w", err)
	}
	if _, err = c.conn.Write(frame); err != nil {
		return fmt.Errorf("writing handshake: %w", err)
	}

	opcode, respData, err := DecodeFrame(c.conn)
	if err != nil {
		return fmt.Errorf("reading handshake response: %w", err)
	}
	if opcode != OpFrame {
		return fmt.Errorf("unexpected handshake response opcode: %d", opcode)
	}

	var resp map[string]any
	if err := json.Unmarshal(respData, &resp); err != nil {
		return fmt.Errorf("parsing handshake response: %w", err)
	}
	if evt, _ := resp["evt"].(string); evt == "ERROR" {
		msg, _ := resp["data"].(map[string]any)["message"].(string)
		return fmt.Errorf("handshake rejected: %s", msg)
	}

	return nil
}

// sendCommand writes a command frame to the IPC connection.
// The caller must hold c.mu.
func (c *Client) sendCommand(cmd string, args map[string]any) error {
	if c.conn == nil {
		return ErrNotConnected
	}

	c.nonce++
	nonce := strconv.FormatUint(c.nonce, 10)

	payload, err := json.Marshal(map[string]any{
		"cmd":   cmd,
		"args":  args,
		"nonce": nonce,
	})
	if err != nil {
		return fmt.Errorf("marshaling command: %w", err)
	}

	frame, err := EncodeFrame(OpFrame, payload)
	if err != nil {
		return fmt.Errorf("encoding command: %w", err)
	}
	if _, err = c.conn.Write(frame); err != nil {
		return fmt.Errorf("writing command: %w", err)
	}
	return nil
}
