package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// newOpenCode describes the OpenCode CLI. Config (providers/baseURL) lives in
// ~/.config/opencode/opencode.json; credentials in
// ~/.local/share/opencode/auth.json (per-provider api key or oauth).
func newOpenCode() *Tool {
	configPath := filepath.Join(home(), ".config", "opencode", "opencode.json")
	authPath := filepath.Join(home(), ".local", "share", "opencode", "auth.json")

	return &Tool{
		Name:            "opencode",
		Title:           "OpenCode",
		Provider:        "openai",
		DefaultEndpoint: "https://api.openai.com/v1",
		Artifacts: []Artifact{
			// opencode.json holds options.apiKey, so keep it private.
			NewFile("opencode.json", configPath, 0o600),
			NewFile("auth.json", authPath, 0o600),
		},
		ApplyAuth: func(a AuthSpec) error {
			// Register an OpenAI-compatible provider "charon" directly in the
			// config. OpenCode needs the key in options.apiKey and a non-empty
			// models map for the models to appear in /models.
			cfg, err := loadJSONMap(configPath)
			if err != nil {
				return err
			}
			if cfg["$schema"] == nil {
				cfg["$schema"] = "https://opencode.ai/config.json"
			}
			provider := subMap(cfg, "provider")
			options := map[string]any{"baseURL": a.Endpoint}
			if a.Key != "" {
				options["apiKey"] = a.Key
			}
			entry := map[string]any{
				"npm":     "@ai-sdk/openai-compatible",
				"name":    "charon",
				"options": options,
			}
			if a.Model != "" {
				entry["models"] = map[string]any{a.Model: map[string]any{"name": a.Model}}
				cfg["model"] = "charon/" + a.Model
			}
			provider["charon"] = entry
			return writeJSONMap(configPath, cfg, 0o600)
		},
		Detected: func() bool {
			_, err := os.Stat(authPath)
			return err == nil
		},
		Describe: func() (Info, error) {
			var info Info

			// A charon-managed provider keeps endpoint + key in the config.
			if data, err := os.ReadFile(configPath); err == nil {
				var cfg struct {
					Provider map[string]struct {
						Options struct {
							BaseURL string `json:"baseURL"`
							APIKey  string `json:"apiKey"`
						} `json:"options"`
					} `json:"provider"`
				}
				if json.Unmarshal(data, &cfg) == nil {
					if p, ok := cfg.Provider["charon"]; ok {
						info.Endpoint = p.Options.BaseURL
						if p.Options.APIKey != "" {
							info.Secret, info.AuthMode = p.Options.APIKey, "api (charon)"
						}
					}
					if info.Endpoint == "" {
						for _, p := range cfg.Provider {
							if p.Options.BaseURL != "" {
								info.Endpoint = p.Options.BaseURL
								break
							}
						}
					}
				}
			}

			// Otherwise fall back to a key stored in auth.json (login-based).
			if info.AuthMode == "" {
				if data, err := os.ReadFile(authPath); err == nil {
					var auth map[string]struct {
						Type string `json:"type"`
						Key  string `json:"key"`
					}
					if json.Unmarshal(data, &auth) == nil {
						for name, p := range auth {
							if p.Type == "api" && p.Key != "" {
								info.AuthMode, info.Secret = "api ("+name+")", p.Key
								break
							}
						}
						if info.AuthMode == "" {
							for name, p := range auth {
								info.AuthMode = p.Type + " (" + name + ")"
								break
							}
						}
					}
				}
			}

			if info.Endpoint == "" {
				info.Endpoint = "(provider default)"
			}
			if info.AuthMode == "" {
				info.AuthMode = "none"
			}
			return info, nil
		},
	}
}
