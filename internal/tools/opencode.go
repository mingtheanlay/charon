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
			NewFile("opencode.json", configPath, 0o644),
			NewFile("auth.json", authPath, 0o600),
		},
		ApplyAuth: func(a AuthSpec) error {
			// Store the key and register an OpenAI-compatible provider "aies".
			auth, err := loadJSONMap(authPath)
			if err != nil {
				return err
			}
			auth["aies"] = map[string]any{"type": "api", "key": a.Key}
			if err := writeJSONMap(authPath, auth, 0o600); err != nil {
				return err
			}

			cfg, err := loadJSONMap(configPath)
			if err != nil {
				return err
			}
			provider := subMap(cfg, "provider")
			entry := map[string]any{
				"npm":     "@ai-sdk/openai-compatible",
				"name":    "aies",
				"options": map[string]any{"baseURL": a.Endpoint},
			}
			if a.Model != "" {
				entry["models"] = map[string]any{a.Model: map[string]any{}}
				cfg["model"] = "charon/" + a.Model
			}
			provider["aies"] = entry
			return writeJSONMap(configPath, cfg, 0o644)
		},
		Detected: func() bool {
			_, err := os.Stat(authPath)
			return err == nil
		},
		Describe: func() (Info, error) {
			var info Info

			if data, err := os.ReadFile(authPath); err == nil {
				var auth map[string]struct {
					Type string `json:"type"`
					Key  string `json:"key"`
				}
				if json.Unmarshal(data, &auth) == nil {
					// Prefer a real api-key provider; else fall back to any entry.
					for name, p := range auth {
						if p.Type == "api" && p.Key != "" {
							info.AuthMode = "api (" + name + ")"
							info.Secret = p.Key
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

			if data, err := os.ReadFile(configPath); err == nil {
				var cfg struct {
					Provider map[string]struct {
						Options struct {
							BaseURL string `json:"baseURL"`
						} `json:"options"`
					} `json:"provider"`
				}
				if json.Unmarshal(data, &cfg) == nil {
					for _, p := range cfg.Provider {
						if p.Options.BaseURL != "" {
							info.Endpoint = p.Options.BaseURL
							break
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
