package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"charon/internal/secret"
)

const claudeKeychainService = "Claude Code-credentials"

// newClaude describes Anthropic's Claude Code CLI. Endpoint/API key live in
// ~/.claude/settings.json (env block); OAuth logins live in the macOS keychain.
func newClaude() *Tool {
	settingsPath := filepath.Join(home(), ".claude", "settings.json")

	return &Tool{
		Name:            "claude",
		Title:           "Claude Code",
		Provider:        "anthropic",
		DefaultEndpoint: "https://api.anthropic.com",
		Artifacts: []Artifact{
			// settings.json may hold a Bearer token for a custom endpoint.
			NewFile("settings.json", settingsPath, 0o600),
			NewKeychain("credentials", claudeKeychainService, os.Getenv("USER")),
		},
		ApplyAuth: func(a AuthSpec) error {
			// settings.json may hold a Bearer token, so keep it private (0600).
			s, err := loadJSONMap(settingsPath)
			if err != nil {
				return err
			}
			env := subMap(s, "env")
			// Start from a clean auth slate so we never send conflicting headers.
			delete(env, "ANTHROPIC_API_KEY")
			delete(env, "ANTHROPIC_AUTH_TOKEN")

			custom := a.Endpoint != "" && !strings.Contains(a.Endpoint, "api.anthropic.com")
			if a.Endpoint != "" {
				env["ANTHROPIC_BASE_URL"] = a.Endpoint
			}
			if custom {
				// Third-party gateways expect Authorization: Bearer <token>.
				env["ANTHROPIC_AUTH_TOKEN"] = a.Key
			} else {
				// Anthropic's own API uses the x-api-key header.
				env["ANTHROPIC_API_KEY"] = a.Key
			}
			if a.Model != "" {
				s["model"] = a.Model // top-level "model" is preferred over env
			}
			return writeJSONMap(settingsPath, s, 0o600)
		},
		Detected: func() bool {
			if _, err := os.Stat(settingsPath); err == nil {
				return true
			}
			_, err := secret.KeychainRead(claudeKeychainService)
			return err == nil
		},
		Describe: func() (Info, error) {
			var info Info

			if data, err := os.ReadFile(settingsPath); err == nil {
				var s struct {
					Model string `json:"model"`
					Env   struct {
						BaseURL   string `json:"ANTHROPIC_BASE_URL"`
						APIKey    string `json:"ANTHROPIC_API_KEY"`
						AuthToken string `json:"ANTHROPIC_AUTH_TOKEN"`
						Model     string `json:"ANTHROPIC_MODEL"`
					} `json:"env"`
				}
				if json.Unmarshal(data, &s) == nil {
					info.Endpoint = s.Env.BaseURL
					info.Model = s.Model
					if info.Model == "" {
						info.Model = s.Env.Model
					}
					if s.Env.AuthToken != "" {
						info.Secret, info.AuthMode = s.Env.AuthToken, "api (bearer)"
					} else if s.Env.APIKey != "" {
						info.Secret, info.AuthMode = s.Env.APIKey, "api"
					}
				}
			}

			if info.AuthMode == "" {
				if _, err := secret.KeychainRead(claudeKeychainService); err == nil {
					info.AuthMode = "oauth"
				}
			}

			if info.Endpoint == "" {
				info.Endpoint = "api.anthropic.com (default)"
			}
			if info.AuthMode == "" {
				info.AuthMode = "none"
			}
			return info, nil
		},
	}
}
