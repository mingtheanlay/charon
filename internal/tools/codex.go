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
			NewFile("config.toml", configPath, 0o644),
			NewFile("auth.json", authPath, 0o600),
		},
		ApplyAuth: func(a AuthSpec) error {
			// Register a custom OpenAI-compatible provider and point Codex at it.
			cfg, err := loadTOMLMap(configPath)
			if err != nil {
				return err
			}
			if a.Model != "" {
				cfg["model"] = a.Model
			}
			cfg["model_provider"] = "aies"
			providers := subMap(cfg, "model_providers")
			providers["aies"] = map[string]any{
				"name":     "aies",
				"base_url": a.Endpoint,
				"env_key":  "OPENAI_API_KEY",
				"wire_api": "chat",
			}
			if err := writeTOMLMap(configPath, cfg, 0o644); err != nil {
				return err
			}

			auth, err := loadJSONMap(authPath)
			if err != nil {
				return err
			}
			auth["OPENAI_API_KEY"] = a.Key
			auth["auth_mode"] = "apikey"
			return writeJSONMap(authPath, auth, 0o600)
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
						BaseURL string `toml:"base_url"`
					} `toml:"model_providers"`
				}
				if toml.Unmarshal(data, &cfg) == nil {
					info.Model = cfg.Model
					if p, ok := cfg.ModelProviders[cfg.ModelProvider]; ok && p.BaseURL != "" {
						info.Endpoint = p.BaseURL
					}
				}
			}

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
