package json

//import (
//	"encoding/json"
//	"fmt"
//	"net/url"
//)
//
//// CredentialsIntoURL modifies a json.RawMessage by merging any "username" and "password"
//// fields into the "url" field, then removing the credential fields from the JSON.
//// Returns the modified JSON or an error if the operation fails.
////
//// If password exists but username doesn't (neither in JSON nor in URL), returns an error.
//func CredentialsIntoURL(raw json.RawMessage) (json.RawMessage, error) {
//	// Unmarshal into a map to manipulate.
//	var data map[string]interface{}
//	if err := json.Unmarshal(raw, &data); err != nil {
//		return nil, fmt.Errorf("unmarshaling json: %w", err)
//	}
//
//	urlStr, urlFound := data["url"].(string)
//	userStr, userFound := data["username"].(string)
//	passStr, passFound := data["password"].(string)
//
//	if userFound != passFound {
//		return nil, fmt.Errorf("json config: username and password must be provided together")
//	}
//	if !(userFound && passFound) {
//		return raw, nil // Nothing to do, return original.
//	}
//	if !urlFound {
//		return nil, fmt.Errorf("json config: url field is required when username and password are provided")
//	}
//
//	// Parse the URL.
//	u, err := url.Parse(urlStr)
//	if err != nil {
//		return nil, fmt.Errorf("json config: parsing url %q: %w", urlStr, err)
//	}
//
//	// Update the URL with credentials from JSON.
//	u.User = url.UserPassword(userStr, passStr)
//
//	// Update the URL in the data map.
//	data["url"] = u.String()
//
//	// Remove the username and password fields from the data map.
//	delete(data, "username")
//	delete(data, "password")
//
//	// Re-marshal to JSON
//	updated, err := json.Marshal(data)
//	if err != nil {
//		return nil, fmt.Errorf("marshaling updated json: %w", err)
//	}
//
//	return updated, nil
//}
