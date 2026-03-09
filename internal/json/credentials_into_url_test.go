package json

//import (
//	"encoding/json"
//	"testing"
//
//	"github.com/stretchr/testify/require"
//)
//
//func TestUpdateJSONWithCredentials(t *testing.T) {
//	tests := map[string]struct {
//		name        string
//		input       string
//		expected    string
//		expectError bool
//	}{
//		"add_username_and_password_to_url": {
//			input:    `{"url":"https://example.com/path","username":"user1","password":"pass1"}`,
//			expected: `{"url":"https://user1:pass1@example.com/path"}`,
//		},
//		"override_existing_credentials_in_url": {
//			input:    `{"url":"https://olduser:oldpass@example.com/path","username":"newuser","password":"newpass"}`,
//			expected: `{"url":"https://newuser:newpass@example.com/path"}`,
//		},
//		"username_without_password_-_error": {
//			input:       `{"url":"https://example.com/path","username":"user1"}`,
//			expectError: true,
//		},
//		"password_without_username_-_error": {
//			input:       `{"url":"https://example.com/path","password":"pass1"}`,
//			expectError: true,
//		},
//		"no_credentials_in_json": {
//			input:    `{"url":"https://example.com/path","other":"value"}`,
//			expected: `{"url":"https://example.com/path","other":"value"}`,
//		},
//		"preserves_other_fields": {
//			input:    `{"url":"https://example.com/path","username":"user1","password":"pass1","api_key":"secret","timeout":30}`,
//			expected: `{"url":"https://user1:pass1@example.com/path","api_key":"secret","timeout":30}`,
//		},
//		"no_url_field": {
//			input:    `{"username":"user1","password":"pass1"}`,
//			expected: `{"username":"user1","password":"pass1"}`,
//		},
//	}
//
//	for tName, tCase := range tests {
//		t.Run(tName, func(t *testing.T) {
//			t.Parallel()
//
//			result, err := CredentialsIntoURL(json.RawMessage(tCase.input))
//
//			if tCase.expectError {
//				if err == nil {
//					t.Errorf("expected error but got none")
//				}
//				return
//			}
//
//			if err != nil {
//				t.Errorf("unexpected error: %v", err)
//				return
//			}
//
//			require.JSONEq(t, tCase.expected, string(result))
//		})
//	}
//}
