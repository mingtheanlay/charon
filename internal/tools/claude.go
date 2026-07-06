package tools

import (
	"encoding/json"
	"os"
	"path/filepath"

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
			NewFile("settings.json", settingsPath, 0o644),
			NewKeychain("credentials", claudeKeychainService, os.Getenv("USER")),
		},
		ApplyAuth: func(a AuthSpec) error {
			s, err := loadJSONMap(settingsPath)
			if err != nil {
				return err
			}
			env := subMap(s, "env")
			if a.Endpoint != "" {
				env["ANTHROPIC_BASE_URL"] = a.Endpoint
			}
			env["ANTHROPIC_API_KEY"] = a.Key
			if a.Model != "" {
				env["ANTHROPIC_MODEL"] = a.Model
			}
			return writeJSONMap(settingsPath, s, 0o644)
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
					Env struct {
						BaseURL   string `json:"ANTHROPIC_BASE_URL"`
						APIKey    string `json:"ANTHROPIC_API_KEY"`
						AuthToken string `json:"ANTHROPIC_AUTH_TOKEN"`
						Model     string `json:"ANTHROPIC_MODEL"`
					} `json:"env"`
				}
				if json.Unmarshal(data, &s) == nil {
					info.Endpoint = s.Env.BaseURL
					info.Model = s.Env.Model
					if s.Env.APIKey != "" {
						info.Secret, info.AuthMode = s.Env.APIKey, "api"
					} else if s.Env.AuthToken != "" {
						info.Secret, info.AuthMode = s.Env.AuthToken, "api"
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
