package message

import "encoding/json"

const (
	TypeChallenge         = "challenge"
	TypeChallengeResponse = "challenge_response"
	TypeConfig            = "configuration"
	TypeError             = "error"
	TypeGoodbye           = "goodbye"
	TypeHello             = "server_hello"
	TypeListening         = "listening"

	TypeDataSourceRequest  = "data_source_request"
	TypeDataSourceResponse = "data_source_response"
	TypeResourceRequest    = "resource_request"
	TypeResourceResponse   = "resource_response"
)

// payloadTypes maps message type strings to their corresponding Go struct types.
// It is referenced during payload unmarshaling to ensure that the caller has
// passed the correct target type for the given message.
var payloadTypes = map[string]any{
	TypeChallenge:         (*Challenge)(nil),
	TypeChallengeResponse: (*ChallengeResponse)(nil),
	TypeConfig:            (*Config)(nil),
	TypeError:             (*Error)(nil),
	TypeGoodbye:           (*Goodbye)(nil),
	TypeHello:             (*Hello)(nil),
	TypeListening:         (*Listening)(nil),

	TypeDataSourceRequest:  (*DataSourceRequest)(nil),
	TypeDataSourceResponse: (*DataSourceResponse)(nil),
	TypeResourceRequest:    (*ResourceRequest)(nil),
	TypeResourceResponse:   (*ResourceResponse)(nil),
}

type Challenge struct {
	Nonce    []byte `json:"nonce"`
	Expected []byte `json:"expected"` // todo - this is only for testing - delete me
}

type ChallengeResponse struct {
	HMAC []byte `json:"hmac"`
}

type Config struct {
	ServerConfig   json.RawMessage `json:"server_config"`
	ProviderConfig json.RawMessage `json:"provider_config"`
}

type Error struct {
	Error string `json:"error"`
}

type Goodbye struct{}

type Hello struct {
	Resources   []string `json:"resources"`
	DataSources []string `json:"data_sources"`
}

type Listening struct {
	AuthNRequired bool   `json:"authentication_required"`
	ListeningOn   string `json:"listening_on"`
}

type DataSourceRequest struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}

type DataSourceResponse struct {
	Name  string          `json:"name"`
	State json.RawMessage `json:"state"`
}

type ResourceRequest struct {
	Name   string          `json:"name"`
	Method string          `json:"method"`
	Config json.RawMessage `json:"config"`
	Plan   json.RawMessage `json:"plan"`
	State  json.RawMessage `json:"state"`
}

type ResourceResponse struct {
	Name  string          `json:"name"`
	State json.RawMessage `json:"state"`
}
