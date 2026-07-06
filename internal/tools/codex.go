package tools

import (
	"encoding/json"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// newCodex describes the OpenAI Codex CLI (~/.codex).
func newCodex() *Tool {
	dir := filepath.Join(home(), ".codex")
	configPath := filepath.Join(dir, "config.toml")
	authPath := filepath.Join(dir, "auth.json")

	return &Tool{
		Name:            "codex",
		Title:           "Codex",
		Provider:        "openai",
		DefaultEndpoint: "https://api.openai.com/v1",
		Artifacts: []Artifact{
			// config.toml holds the inline bearer token, so keep it private.
			NewFile("config.toml", configPath, 0o600),
			NewFile("auth.json", authPath, 0o600),
		},
		ApplyAuth: func(a AuthSpec) error {
			// Register a custom OpenAI-compatible provider and point Codex at it.
			// Codex's env_key reads the key from a runtime env var, so instead we
			// embed it inline via experimental_bearer_token to be self-contained.
			// auth.json (ChatGPT OAuth) is left untouched.
			cfg, err := loadTOMLMap(configPath)
			if err != nil {
				return err
			}
			if a.Model != "" {
				cfg["model"] = a.Model
			}
			cfg["model_provider"] = "charon"
			providers := subMap(cfg, "model_providers")
			providers["charon"] = map[string]any{
				"name":     "charon",
				"base_url": a.Endpoint,
				// Codex removed "chat" in Feb 2026; "responses" is the only
				// supported wire API. See openai/codex discussion #7782.
				"wire_api":                  "responses",
				"experimental_bearer_token": a.Key,
			}
			return writeTOMLMap(configPath, cfg, 0o600)
		},
		Detected: func() bool {
			_, err := os.Stat(authPath)
			return err == nil
		},
		Describe: func() (Info, error) {
			var info Info

			if data, err := os.ReadFile(configPath); err == nil {
				var cfg struct {
					Model          string `toml:"model"`
					ModelProvider  string `toml:"model_provider"`
					ModelProviders map[string]struct {
						BaseURL     string `toml:"base_url"`
						BearerToken string `toml:"experimental_bearer_token"`
					} `toml:"model_providers"`
				}
				if toml.Unmarshal(data, &cfg) == nil {
					info.Model = cfg.Model
					if p, ok := cfg.ModelProviders[cfg.ModelProvider]; ok {
						if p.BaseURL != "" {
							info.Endpoint = p.BaseURL
						}
						if p.BearerToken != "" {
							info.Secret, info.AuthMode = p.BearerToken, "api"
						}
					}
				}
			}

			if info.AuthMode == "" {
				if data, err := os.ReadFile(authPath); err == nil {
					var auth struct {
						AuthMode string `json:"auth_mode"`
						APIKey   string `json:"OPENAI_API_KEY"`
					}
					if json.Unmarshal(data, &auth) == nil {
						info.AuthMode = auth.AuthMode
						info.Secret = auth.APIKey
						if info.AuthMode == "" && auth.APIKey != "" {
							info.AuthMode = "api"
						}
					}
				}
			}

			if info.Endpoint == "" {
				info.Endpoint = "api.openai.com (default)"
			}
			if info.AuthMode == "" {
				info.AuthMode = "none"
			}
			return info, nil
		},
	}
}
