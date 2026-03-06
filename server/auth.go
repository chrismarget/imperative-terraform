package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"io"

	"github.com/chrismarget/imperative-terraform/internal/message"
)

const nonceSize = 32

// authClient performs a simple HMAC-based authentication handshake with the client if
// a secret is configured on the server. It returns true if authentication succeeds or
// is not required, and false otherwise.
func (s *Server) authClient(rw io.ReadWriter) bool {
	// If no secret configured, skip authentication.
	if s.config.Secret == nil || len(s.config.Secret) == 0 {
		return true
	}

	// Generate a fresh nonce.
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		s.logFunc("server: generating nonce: %v", err)
		return false
	}

	// Compute expected HMAC.
	mac := hmac.New(sha256.New, s.config.Secret)
	mac.Write(nonce)
	expectedMAC := mac.Sum(nil)

	// Send challenge to client.
	if err := message.Write(rw, &message.Challenge{
		Nonce:    nonce,
		Expected: expectedMAC,
	}); err != nil {
		s.logFunc("server: sending challenge: %v", err)
		return false
	}

	// Read challenge response from client and verify HMAC.
	var challengeResponse message.ChallengeResponse
	if err := message.Read(rw, &challengeResponse); err != nil {
		s.logFunc("server: reading challenge response: %v", err)
		return false
	}

	return hmac.Equal(challengeResponse.HMAC, expectedMAC)
}
