package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// claudeContextWindow returns the window to pin for a Claude model (200K), else 0.
// Conservative: avoids the 1M beta window; OpenAI slugs Codex already sizes itself.
func claudeContextWindow(model string) int {
	if strings.Contains(strings.ToLower(model), "claude") {
		return 200_000
	}
	return 0
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
			NewFile("config.toml", configPath, 0o600), // holds the inline bearer token
			NewFile("auth.json", authPath, 0o600),
		},
		ApplyAuth: func(a AuthSpec) error {
			// Register a self-contained OpenAI-compatible provider (key embedded inline)
			// and point Codex at it; auth.json (ChatGPT OAuth) is left untouched.
			cfg, err := loadTOMLMap(configPath)
			if err != nil {
				return err
			}
			if a.Model != "" {
				cfg["model"] = a.Model
			}
			// Codex sizes unknown (non-OpenAI) slugs at 272K > Claude's real 200K and overruns
			// the context; pin the window for Claude models, clearing any stale prior value.
			delete(cfg, "model_context_window")
			if w := claudeContextWindow(a.Model); w != 0 {
				cfg["model_context_window"] = w
			}
			cfg["model_provider"] = "charon"
			providers := subMap(cfg, "model_providers")
			original := snapshotProviders(providers) // guard: write may only touch "charon"
			providers["charon"] = map[string]any{
				"name":     "charon",
				"base_url": a.Endpoint,
				// "responses" is the only wire API since Codex dropped "chat" (openai/codex #7782).
				"wire_api":                  "responses",
				"experimental_bearer_token": a.Key,
			}
			if err := ensureOnlyCharonChanged(original, providers); err != nil {
				return err
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
						info.Secret = auth.APIKey
						// Codex writes "chatgpt" for an OAuth login and "apikey" for a key;
						// present them as the friendlier "oauth"/"api".
						switch auth.AuthMode {
						case "chatgpt":
							info.AuthMode = "oauth"
						case "apikey":
							info.AuthMode = "api"
						default:
							info.AuthMode = auth.AuthMode
						}
						if info.AuthMode == "" && auth.APIKey != "" {
							info.AuthMode = "api"
						}
					}
				}
			}

			return info.withDefaults("api.openai.com (default)"), nil
		},
	}
}
