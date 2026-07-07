package tools

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// decodeJWTEmail reads the "email" claim from a JWT's payload without verifying
// its signature — the token is already local and we only want a display name.
// It never errors: any malformed input yields "".
func decodeJWTEmail(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	// JWT payloads are base64url with no padding.
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(data, &claims) != nil {
		return ""
	}
	return claims.Email
}
