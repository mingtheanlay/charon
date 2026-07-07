package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// opencodeConfigPath returns the existing config (opencode.jsonc, else legacy
// opencode.json) so charon edits it in place, defaulting to opencode.jsonc.
func opencodeConfigPath() string {
	dir := filepath.Join(home(), ".config", "opencode")
	for _, name := range []string{"opencode.jsonc", "opencode.json"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(dir, "opencode.jsonc")
}

// newOpenCode describes OpenCode: providers in ~/.config/opencode/opencode.jsonc,
// credentials in ~/.local/share/opencode/auth.json.
func newOpenCode() *Tool {
	configPath := opencodeConfigPath()
	authPath := filepath.Join(home(), ".local", "share", "opencode", "auth.json")

	return &Tool{
		Name:            "opencode",
		Title:           "OpenCode",
		Provider:        "openai",
		DefaultEndpoint: "https://api.openai.com/v1",
		Artifacts: []Artifact{
			// The config holds provider options.apiKey, so keep it private.
			NewFile(filepath.Base(configPath), configPath, 0o600), // holds options.apiKey
			NewFile("auth.json", authPath, 0o600),
		},
		ApplyAuth: func(a AuthSpec) error {
			// Register a "charon" provider: OpenCode needs options.apiKey and a
			// non-empty models map for the models to show in /models.
			cfg, err := loadJSONMap(configPath)
			if err != nil {
				return err
			}
			if cfg["$schema"] == nil {
				cfg["$schema"] = "https://opencode.ai/config.json"
			}
			provider := subMap(cfg, "provider")
			original := snapshotProviders(provider) // guard: write may only touch "charon"

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

			// Fail loudly rather than risk clobbering a user-authored provider.
			if err := ensureOnlyCharonChanged(original, provider); err != nil {
				return err
			}
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
							info.Secret, info.AuthMode = p.Options.APIKey, "api"
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

			return info.withDefaults("(provider default)"), nil
		},
	}
}
