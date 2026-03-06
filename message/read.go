package message

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/chrismarget/imperative-terraform/internal/pointer"
)

// Read reads a JSON message from r, validates the message type matches the target struct,
// then unmarshals the payload into target.
func Read(r io.Reader, target any) error {
	if msg, ok := target.(*Message); ok {
		return decodeEnvelope(r, msg)
	}

	// Decode the message from the socket
	var msg Message
	if err := json.NewDecoder(r).Decode(&msg); err != nil {
		return fmt.Errorf("read: decoding message: %w", err)
	}

	// Validate protocol version
	if msg.ProtocolVersion == nil || *msg.ProtocolVersion != protocolVersion {
		return fmt.Errorf("read: unsupported protocol version: %s", pointer.ValStr(msg.ProtocolVersion))
	}

	// Get the expected payload type for this message type
	msgPayloadType, ok := payloadTypes[msg.Type]
	if !ok {
		return fmt.Errorf("read: unknown message type: %q", msg.Type)
	}

	// Validate target type matches expected type for this message
	if reflect.TypeOf(target) != reflect.TypeOf(msgPayloadType) {
		return fmt.Errorf("read: message type %q expects %T, got %T", msg.Type, msgPayloadType, target)
	}

	return UnpackPayload(target, msg.Payload)
}

// UnpackPayload unmarshals the inner message payload into target.
func UnpackPayload(target any, raw json.RawMessage) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("read: unmarshaling payload: %w", err)
	}

	return nil
}

func decodeEnvelope(r io.Reader, msg *Message) error {
	if err := json.NewDecoder(r).Decode(msg); err != nil {
		return fmt.Errorf("read: decoding message from socket: %w", err)
	}

	// Validate protocol version
	if msg.ProtocolVersion == nil || *msg.ProtocolVersion != protocolVersion {
		return fmt.Errorf("read: unsupported protocol version: %s", pointer.ValStr(msg.ProtocolVersion))
	}

	return nil
}
