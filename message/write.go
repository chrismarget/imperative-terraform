package message

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/chrismarget/imperative-terraform/internal/pointer"
)

const protocolVersion = 1

// typeToMessageType is the reverse lookup map: payload type -> message type string.
// It is built automatically from payloadTypes at init time.
var typeToMessageType map[reflect.Type]string

func init() {
	typeToMessageType = make(map[reflect.Type]string, len(payloadTypes))
	for msgType, payloadPtr := range payloadTypes {
		typeToMessageType[reflect.TypeOf(payloadPtr)] = msgType
	}
}

// Write marshals a payload struct into a Message with the correct message_type and
// protocol_version, and writes it as JSON to w.
func Write(w io.Writer, payload any) error {
	// Get the type of the payload (as a pointer, since that's how we store them)
	payloadType := reflect.TypeOf(payload)

	// If they passed a value, convert to pointer type for lookup
	if payloadType.Kind() != reflect.Ptr {
		// Create a pointer to the value's type for lookup
		payloadType = reflect.PointerTo(payloadType)
	}

	// Look up the message type string
	msgTypeStr, ok := typeToMessageType[payloadType]
	if !ok {
		return fmt.Errorf("write: unknown payload type: %T", payload)
	}

	// Marshal the payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("write: marshaling payload: %w", err)
	}

	// Build the complete message with protocol version set
	msg := Message{
		Type:            msgTypeStr,
		ProtocolVersion: pointer.To(protocolVersion),
		Payload:         payloadBytes,
	}

	// Write the message to the socket
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(msg); err != nil {
		return fmt.Errorf("write: encoding message: %w", err)
	}

	return nil
}
