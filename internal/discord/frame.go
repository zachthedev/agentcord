package discord

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ///////////////////////////////////////////////
// Constants
// ///////////////////////////////////////////////

// Opcode represents a Discord IPC frame opcode.
type Opcode uint32

const (
	// OpHandshake is the opcode for the initial IPC handshake.
	OpHandshake Opcode = 0
	// OpFrame is the opcode for a standard IPC data frame.
	OpFrame Opcode = 1
	// OpClose is the opcode for closing the IPC connection.
	OpClose Opcode = 2

	// frameHeaderSize is the byte length of the IPC frame header
	// consisting of a 4-byte little-endian opcode followed by a
	// 4-byte little-endian payload length.
	frameHeaderSize = 8

	// MaxPayloadSize is the maximum allowed payload size (1 MB).
	MaxPayloadSize = 1 << 20

	// maxIPCSlots is the number of IPC socket slots Discord may listen on (0-9).
	maxIPCSlots = 10
)

// ErrPayloadTooLarge is returned when a received frame payload exceeds MaxPayloadSize.
var ErrPayloadTooLarge = errors.New("payload too large")

// ErrIPCNotAvailable is returned when no Discord IPC socket can be reached.
var ErrIPCNotAvailable = errors.New("discord IPC not available")

// ///////////////////////////////////////////////
// Frame Encoding
// ///////////////////////////////////////////////

// EncodeFrame builds a Discord IPC frame: [4-byte LE opcode][4-byte LE length][payload].
func EncodeFrame(opcode Opcode, payload []byte) ([]byte, error) {
	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf("%w: %d bytes (max %d)", ErrPayloadTooLarge, len(payload), MaxPayloadSize)
	}
	frame := make([]byte, frameHeaderSize+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(opcode))
	binary.LittleEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:], payload)
	return frame, nil
}

// ///////////////////////////////////////////////
// Frame Decoding
// ///////////////////////////////////////////////

// DecodeFrame reads a single Discord IPC frame from reader.
// It handles partial reads via io.ReadFull.
func DecodeFrame(reader io.Reader) (opcode Opcode, payload []byte, err error) {
	header := make([]byte, frameHeaderSize)
	if _, err = io.ReadFull(reader, header); err != nil {
		return 0, nil, fmt.Errorf("reading frame header: %w", err)
	}

	opcode = Opcode(binary.LittleEndian.Uint32(header[0:4]))
	length := binary.LittleEndian.Uint32(header[4:8])

	if length > MaxPayloadSize {
		return 0, nil, fmt.Errorf("%w: %d bytes (max %d)", ErrPayloadTooLarge, length, MaxPayloadSize)
	}

	payload = make([]byte, length)
	if _, err = io.ReadFull(reader, payload); err != nil {
		return 0, nil, fmt.Errorf("reading frame payload: %w", err)
	}

	return opcode, payload, nil
}
