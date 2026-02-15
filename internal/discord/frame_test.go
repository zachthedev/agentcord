// Tests for [EncodeFrame] and [DecodeFrame] covering round-trip encoding,
// partial reads, multiple sequential frames, and error cases.
package discord

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// ///////////////////////////////////////////////
// EncodeFrame
// ///////////////////////////////////////////////

func TestEncodeFrame(t *testing.T) {
	payload := []byte(`{"v":1,"client_id":"12345"}`)
	frame, err := EncodeFrame(OpHandshake, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(frame) != 8+len(payload) {
		t.Fatalf("expected frame length %d, got %d", 8+len(payload), len(frame))
	}

	opcode := Opcode(binary.LittleEndian.Uint32(frame[0:4]))
	if opcode != OpHandshake {
		t.Fatalf("expected opcode %d, got %d", OpHandshake, opcode)
	}

	length := binary.LittleEndian.Uint32(frame[4:8])
	if length != uint32(len(payload)) {
		t.Fatalf("expected length %d, got %d", len(payload), length)
	}

	if !bytes.Equal(frame[8:], payload) {
		t.Fatalf("payload mismatch: expected %q, got %q", payload, frame[8:])
	}
}

func TestEncodeFrame_Oversized(t *testing.T) {
	oversized := make([]byte, MaxPayloadSize+1)
	_, err := EncodeFrame(OpFrame, oversized)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "payload too large") {
		t.Fatalf("expected 'payload too large' error, got: %v", err)
	}
}

// ///////////////////////////////////////////////
// DecodeFrame
// ///////////////////////////////////////////////

func mustEncodeFrame(t *testing.T, opcode Opcode, payload []byte) []byte {
	t.Helper()
	frame, err := EncodeFrame(opcode, payload)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}
	return frame
}

func TestDecodeFrame(t *testing.T) {
	original := []byte(`{"cmd":"SET_ACTIVITY","args":{}}`)
	encoded := mustEncodeFrame(t, OpFrame, original)

	opcode, payload, err := DecodeFrame(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opcode != OpFrame {
		t.Fatalf("expected opcode %d, got %d", OpFrame, opcode)
	}
	if !bytes.Equal(payload, original) {
		t.Fatalf("payload mismatch: expected %q, got %q", original, payload)
	}
}

func TestDecodeFrame_Partial(t *testing.T) {
	// Use a reader that returns one byte at a time to test partial read handling.
	original := []byte(`{"hello":"world"}`)
	encoded := mustEncodeFrame(t, OpHandshake, original)

	reader := &slowReader{data: encoded}
	opcode, payload, err := DecodeFrame(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opcode != OpHandshake {
		t.Fatalf("expected opcode %d, got %d", OpHandshake, opcode)
	}
	if !bytes.Equal(payload, original) {
		t.Fatalf("payload mismatch: expected %q, got %q", original, payload)
	}
}

// slowReader returns data one byte at a time, simulating partial reads.
type slowReader struct {
	data []byte
	pos  int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}

func TestDecodeFrame_Multiple(t *testing.T) {
	var buf bytes.Buffer

	payloads := []struct {
		name    string
		opcode  Opcode
		payload []byte
	}{
		{"handshake", OpHandshake, []byte(`{"v":1}`)},
		{"set_activity", OpFrame, []byte(`{"cmd":"SET_ACTIVITY"}`)},
		{"close", OpClose, []byte(`{"code":1000}`)},
	}

	for _, p := range payloads {
		buf.Write(mustEncodeFrame(t, p.opcode, p.payload))
	}

	reader := &buf
	for i, expected := range payloads {
		t.Run(fmt.Sprintf("frame_%d_%s", i, expected.name), func(t *testing.T) {
			opcode, payload, err := DecodeFrame(reader)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if opcode != expected.opcode {
				t.Fatalf("expected opcode %d, got %d", expected.opcode, opcode)
			}
			if !bytes.Equal(payload, expected.payload) {
				t.Fatalf("payload mismatch: expected %q, got %q", expected.payload, payload)
			}
		})
	}
}

// ///////////////////////////////////////////////
// DecodeFrame Error Cases
// ///////////////////////////////////////////////

func TestDecodeFrame_Oversized(t *testing.T) {
	// Craft a header claiming a payload larger than MaxPayloadSize.
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header[0:4], uint32(OpFrame))
	binary.LittleEndian.PutUint32(header[4:8], MaxPayloadSize+1)

	_, _, err := DecodeFrame(bytes.NewReader(header))
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "payload too large") {
		t.Fatalf("expected 'payload too large' error, got: %v", err)
	}
}

func TestDecodeFrame_EmptyPayload(t *testing.T) {
	encoded := mustEncodeFrame(t, OpFrame, []byte{})

	opcode, payload, err := DecodeFrame(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opcode != OpFrame {
		t.Fatalf("expected opcode %d, got %d", OpFrame, opcode)
	}
	if len(payload) != 0 {
		t.Fatalf("expected empty payload, got %d bytes", len(payload))
	}
}

func TestDecodeFrame_TruncatedHeader(t *testing.T) {
	// Only 4 bytes instead of the required 8-byte header.
	_, _, err := DecodeFrame(bytes.NewReader([]byte{0, 0, 0, 0}))
	if err == nil {
		t.Fatal("expected error for truncated header")
	}
}

func TestDecodeFrame_TruncatedPayload(t *testing.T) {
	// Header claims 100 bytes but only 5 bytes follow.
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header[0:4], uint32(OpFrame))
	binary.LittleEndian.PutUint32(header[4:8], 100)

	data := append(header, []byte("short")...)
	_, _, err := DecodeFrame(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

// ///////////////////////////////////////////////
// EncodeFrame Size Guard (additional)
// ///////////////////////////////////////////////

func TestEncodeFrame_ExactMax(t *testing.T) {
	payload := make([]byte, MaxPayloadSize)
	_, err := EncodeFrame(OpFrame, payload)
	if err != nil {
		t.Fatalf("expected no error for exactly MaxPayloadSize, got: %v", err)
	}
}

func TestEncodeFrame_EmptyPayload(t *testing.T) {
	frame, err := EncodeFrame(OpFrame, []byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(frame) != frameHeaderSize {
		t.Fatalf("expected frame length %d, got %d", frameHeaderSize, len(frame))
	}
}

func TestEncodeFrame_OversizedWrapsError(t *testing.T) {
	oversized := make([]byte, MaxPayloadSize+100)
	_, err := EncodeFrame(OpFrame, oversized)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("expected ErrPayloadTooLarge, got: %v", err)
	}
}

// ///////////////////////////////////////////////
// Round-trip: EncodeFrame -> DecodeFrame
// ///////////////////////////////////////////////

func TestFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		opcode  Opcode
		payload []byte
	}{
		{"handshake", OpHandshake, []byte(`{"v":1,"client_id":"12345"}`)},
		{"frame_json", OpFrame, []byte(`{"cmd":"SET_ACTIVITY","args":{"pid":1234}}`)},
		{"close", OpClose, []byte(`{"code":1000,"reason":"goodbye"}`)},
		{"empty_payload", OpFrame, []byte{}},
		{"binary_payload", OpHandshake, []byte{0x00, 0xFF, 0xFE, 0x01, 0x80}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := EncodeFrame(tt.opcode, tt.payload)
			if err != nil {
				t.Fatalf("EncodeFrame: %v", err)
			}

			opcode, payload, err := DecodeFrame(bytes.NewReader(frame))
			if err != nil {
				t.Fatalf("DecodeFrame: %v", err)
			}
			if opcode != tt.opcode {
				t.Errorf("opcode = %d, want %d", opcode, tt.opcode)
			}
			if !bytes.Equal(payload, tt.payload) {
				t.Errorf("payload mismatch: got %v, want %v", payload, tt.payload)
			}
		})
	}
}

func TestFrameRoundTrip_AllOpcodes(t *testing.T) {
	opcodes := []Opcode{OpHandshake, OpFrame, OpClose}
	payload := []byte(`{"test":"data"}`)

	for _, op := range opcodes {
		t.Run(fmt.Sprintf("opcode_%d", op), func(t *testing.T) {
			frame, err := EncodeFrame(op, payload)
			if err != nil {
				t.Fatalf("EncodeFrame: %v", err)
			}
			gotOp, gotPayload, err := DecodeFrame(bytes.NewReader(frame))
			if err != nil {
				t.Fatalf("DecodeFrame: %v", err)
			}
			if gotOp != op {
				t.Errorf("opcode = %d, want %d", gotOp, op)
			}
			if !bytes.Equal(gotPayload, payload) {
				t.Errorf("payload mismatch")
			}
		})
	}
}
