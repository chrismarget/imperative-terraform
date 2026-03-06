package message

import "encoding/json"

// Message is the generic message structure used for all protocol messages.
// All messages sent between the imperative server and its clients use this
// structure with a message_type discriminator and an optional payload.
type Message struct {
	Type            string          `json:"type"`
	ProtocolVersion *int            `json:"protocol_version"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}
