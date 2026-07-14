package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charon/internal/artifact"
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
	dataDir := filepath.Join(home(), ".local", "share", "opencode")
	authPath := filepath.Join(dataDir, "auth.json")

	return &Tool{
		Name:            "opencode",
		Title:           "OpenCode",
		Provider:        "openai",
		DefaultEndpoint: "https://api.openai.com/v1",
		Artifacts: []artifact.Artifact{
			// The config holds provider options.apiKey, so keep it private. Other top-level
			// settings (e.g. theme) are CLI preferences, not per-profile auth — preserved live.
			artifact.NewMergedJSONFile(filepath.Base(configPath), configPath, 0o600, "provider", "model", "small_model", "reasoningEffort", "agent", "agents").
				WithDisplay("model", "reasoningEffort").
				WithAgentFallback("agent", "agents"),
			artifact.NewRotatingFile("auth.json", authPath, 0o600), // OAuth logins (e.g. github-copilot); OpenCode refreshes them in place
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

			// Preserve the previously-registered model list (e.g. from an earlier fetch)
			// when this call doesn't bring its own — otherwise an edit that doesn't touch
			// the model field (rename, key rotation, CLI --model) would collapse OpenCode's
			// /models picker down to just the single current model.
			var existingModels []string
			if prev, ok := provider["charon"].(map[string]any); ok {
				if models, ok := prev["models"].(map[string]any); ok {
					for id := range models {
						existingModels = append(existingModels, id)
					}
				}
			}

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
				// Register every model the caller already fetched (e.g. the TUI wizard's
				// picker list), not just a.Model, so OpenCode's own /models picker can
				// switch between them without re-adding the profile. Falls back to the
				// previously-registered list, then just a.Model, when the caller has no
				// fetched list (e.g. the CLI --model flag or an edit of another field).
				ids := a.AllModels
				if len(ids) == 0 {
					ids = existingModels
				}
				if len(ids) == 0 {
					ids = []string{a.Model}
				}
				modelMap := make(map[string]any, len(ids))
				for _, id := range ids {
					modelMap[id] = map[string]any{"name": id}
				}
				entry["models"] = modelMap
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
			return detected("opencode", configPath, authPath)
		},
		Describe: func() (Info, error) {
			var info Info

			// A charon-managed provider keeps endpoint + key in the config.
			if data, err := os.ReadFile(configPath); err == nil {
				var cfg struct {
					Model           string `json:"model"`
					SmallModel      string `json:"small_model"`
					ReasoningEffort string `json:"reasoningEffort"`
					Agent           map[string]struct {
						Model           string `json:"model"`
						ReasoningEffort string `json:"reasoningEffort"`
					} `json:"agent"`
					Agents map[string]struct {
						Model           string `json:"model"`
						ReasoningEffort string `json:"reasoningEffort"`
					} `json:"agents"`
					Provider map[string]struct {
						Options struct {
							BaseURL string `json:"baseURL"`
							APIKey  string `json:"apiKey"`
						} `json:"options"`
					} `json:"provider"`
				}
				if json.Unmarshal(data, &cfg) == nil {
					info.Model = strings.TrimPrefix(cfg.Model, "charon/")
					if info.Model == "" {
						info.Model = strings.TrimPrefix(cfg.SmallModel, "charon/")
					}
					info.Effort = cfg.ReasoningEffort

					// Fallback to agent-specific configs
					if info.Model == "" {
						for _, agent := range cfg.Agents {
							if agent.Model != "" {
								info.Model = strings.TrimPrefix(agent.Model, "charon/")
								break
							}
						}
					}
					if info.Model == "" && cfg.Agent != nil {
						for _, agent := range cfg.Agent {
							if agent.Model != "" {
								info.Model = strings.TrimPrefix(agent.Model, "charon/")
								break
							}
						}
					}

					if info.Effort == "" {
						for _, agent := range cfg.Agents {
							if agent.ReasoningEffort != "" {
								info.Effort = agent.ReasoningEffort
								break
							}
						}
					}
					if info.Effort == "" && cfg.Agent != nil {
						for _, agent := range cfg.Agent {
							if agent.ReasoningEffort != "" {
								info.Effort = agent.ReasoningEffort
								break
							}
						}
					}
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
			if data, err := os.ReadFile(authPath); err == nil {
				var auth map[string]struct {
					Type   string `json:"type"`
					Key    string `json:"key"`
					Access string `json:"access"` // oauth: access token, sometimes a JWT (e.g. ChatGPT via /connect)
				}
				if json.Unmarshal(data, &auth) == nil {
					names := make([]string, 0, len(auth))
					for name := range auth {
						names = append(names, name)
					}
					sort.Strings(names) // map iteration order is random; keep this deterministic

					if info.AuthMode == "" {
						for _, name := range names {
							if p := auth[name]; p.Type == "api" && p.Key != "" {
								info.AuthMode, info.Secret = "api ("+name+")", p.Key
								break
							}
						}
						if info.AuthMode == "" {
							for _, name := range names {
								info.AuthMode = auth[name].Type + " (" + name + ")"
								break
							}
						}
					}

					// An oauth login's account identity must be detected independent of
					// which entry AuthMode picked above — e.g. an unrelated "opencode" api
					// key entry shouldn't hide a real ChatGPT/GitHub login for backup
					// purposes. Prefer a JWT-decodable email (ChatGPT via /connect) over a
					// bare provider name (e.g. github-copilot, whose token isn't a JWT).
					for _, name := range names {
						p := auth[name]
						if p.Type != "oauth" {
							continue
						}
						if info.Account == "" {
							info.Account = name
						}
						if email := decodeJWTEmail(p.Access); email != "" {
							info.Account = email
							break
						}
					}
				}
			}

			return info.withDefaults("(provider default)"), nil
		},
	}
}
