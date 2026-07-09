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
		Email   string `json:"email"`
		Profile struct {
			Email string `json:"email"`
		} `json:"https://api.openai.com/profile"`
	}
	if json.Unmarshal(data, &claims) != nil {
		return ""
	}
	if claims.Email != "" {
		return claims.Email
	}
	// An OpenAI id_token carries a top-level "email" claim, but its access_token (what
	// OpenCode's ChatGPT /connect stores) nests it under this OIDC profile claim instead.
	return claims.Profile.Email
}
