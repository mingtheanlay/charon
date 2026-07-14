package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"charon/internal/artifact"
)

// Pi has no static provider-config file: providers are registered by TypeScript
// extensions (pi.registerProvider(...)) auto-loaded from ~/.pi/agent/extensions.
// charon owns one such extension, charon.ts, wrapping a JSON blob so it can be
// round-tripped without a TS parser; see piExtensionOpen/Close markers below.
const (
	piExtensionOpen  = `pi.registerProvider("charon", `
	piExtensionClose = `);
  // charon:config:end
}
`
)

var piConfigRE = regexp.MustCompile(`(?s)pi\.registerProvider\("charon",\s*(.*?)\);\s*\n\s*// charon:config:end`)

// piModel is one entry of a pi provider's "models" array.
type piModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Reasoning     bool     `json:"reasoning"`
	Input         []string `json:"input"`
	Cost          piCost   `json:"cost"`
	ContextWindow int      `json:"contextWindow"`
	MaxTokens     int      `json:"maxTokens"`
}

type piCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// piProviderConfig is the object literal passed to pi.registerProvider.
type piProviderConfig struct {
	Name    string    `json:"name"`
	BaseURL string    `json:"baseUrl"`
	APIKey  string    `json:"apiKey"`
	API     string    `json:"api"`
	Models  []piModel `json:"models"`
}

// piEscapeValue escapes "$" and "!", which pi's apiKey/headers fields treat as
// env-var interpolation ("$VAR", "${VAR}") and command execution ("!cmd") markers,
// so a literal key/header value containing either is never misinterpreted.
func piEscapeValue(s string) string {
	s = strings.ReplaceAll(s, "$", "$$")
	s = strings.ReplaceAll(s, "!", "$!")
	return s
}

// piContextWindow mirrors claudeContextWindow's Claude-model special-case, plus a
// generic default for everything else.
func piContextWindow(model string) int {
	if w := claudeContextWindow(model); w != 0 {
		return w
	}
	return 128_000
}

// piBuildModels turns a list of model ids into pi model entries.
func piBuildModels(ids []string) []piModel {
	models := make([]piModel, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		models = append(models, piModel{
			ID:            id,
			Name:          id,
			Input:         []string{"text", "image"},
			ContextWindow: piContextWindow(id),
			MaxTokens:     8192,
		})
	}
	return models
}

// piExtensionContent renders charon's extension .ts file for cfg.
func piExtensionContent(cfg piProviderConfig) ([]byte, error) {
	body, err := json.MarshalIndent(cfg, "  ", "  ")
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString(`import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export default function (pi: ExtensionAPI) {
  // charon:config
  `)
	b.WriteString(piExtensionOpen)
	b.Write(body)
	b.WriteString(piExtensionClose)
	return []byte(b.String()), nil
}

// piParseExtension extracts the provider config JSON from a previously written
// charon.ts, "" fields / nil models if the file is absent or unrecognized.
func piParseExtension(data []byte) (piProviderConfig, bool) {
	m := piConfigRE.FindSubmatch(data)
	if m == nil {
		return piProviderConfig{}, false
	}
	var cfg piProviderConfig
	if json.Unmarshal(m[1], &cfg) != nil {
		return piProviderConfig{}, false
	}
	return cfg, true
}

// newPi describes the pi coding agent: providers are registered via a TypeScript
// extension (~/.pi/agent/extensions/charon.ts); model/effort defaults live in
// ~/.pi/agent/settings.json; OAuth-based provider logins (unrelated to charon's
// key-based provider) persist in ~/.pi/agent/auth.json.
func newPi() *Tool {
	dir := filepath.Join(home(), ".pi", "agent")
	settingsPath := filepath.Join(dir, "settings.json")
	authPath := filepath.Join(dir, "auth.json")
	extensionPath := filepath.Join(dir, "extensions", "charon.ts")

	return &Tool{
		Name:            "pi",
		Title:           "Pi",
		Provider:        "openai",
		DefaultEndpoint: "https://api.openai.com/v1",
		Artifacts: []artifact.Artifact{
			// Other settings.json fields (theme, extensions list, shell, ...) are CLI
			// preferences, not per-profile auth — preserved live. defaultModel and
			// defaultThinkingLevel switch with the profile, matching Claude Code/Codex.
			artifact.NewMergedJSONFile("settings.json", settingsPath, 0o600,
				"defaultProvider", "defaultModel", "defaultThinkingLevel").
				WithDisplay("defaultModel", "defaultThinkingLevel"),
			artifact.NewFile("charon.ts", extensionPath, 0o600),    // charon owns this extension file outright
			artifact.NewRotatingFile("auth.json", authPath, 0o600), // OAuth provider logins; pi refreshes them in place
		},
		ApplyAuth: func(a AuthSpec) error {
			// Preserve the previously-registered model list when this call doesn't bring
			// its own (rename, key rotation, CLI --model) — otherwise pi's /model picker
			// would collapse down to just the single current model.
			var existingModels []string
			if data, err := os.ReadFile(extensionPath); err == nil {
				if prev, ok := piParseExtension(data); ok {
					for _, m := range prev.Models {
						existingModels = append(existingModels, m.ID)
					}
				}
			}
			ids := a.AllModels
			if len(ids) == 0 {
				ids = existingModels
			}
			if len(ids) == 0 && a.Model != "" {
				ids = []string{a.Model}
			}

			cfg := piProviderConfig{
				Name:    "charon",
				BaseURL: a.Endpoint,
				APIKey:  piEscapeValue(a.Key),
				API:     "openai-completions",
				Models:  piBuildModels(ids),
			}
			content, err := piExtensionContent(cfg)
			if err != nil {
				return fmt.Errorf("render pi extension: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(extensionPath), 0o700); err != nil {
				return err
			}
			if err := artifact.AtomicWrite(extensionPath, content, 0o600); err != nil {
				return err
			}

			s, err := loadJSONMap(settingsPath)
			if err != nil {
				return err
			}
			s["defaultProvider"] = "charon"
			if a.Model != "" {
				s["defaultModel"] = a.Model
			}
			return writeJSONMap(settingsPath, s, 0o600)
		},
		Detected: func() bool {
			return detected("pi", dir)
		},
		Describe: func() (Info, error) {
			var info Info

			if data, err := os.ReadFile(settingsPath); err == nil {
				var s struct {
					DefaultProvider      string `json:"defaultProvider"`
					DefaultModel         string `json:"defaultModel"`
					DefaultThinkingLevel string `json:"defaultThinkingLevel"`
				}
				if json.Unmarshal(data, &s) == nil {
					info.Model = s.DefaultModel
					info.Effort = s.DefaultThinkingLevel

					if s.DefaultProvider == "charon" || s.DefaultProvider == "" {
						if data, err := os.ReadFile(extensionPath); err == nil {
							if cfg, ok := piParseExtension(data); ok {
								info.Endpoint = cfg.BaseURL
								if cfg.APIKey != "" {
									info.Secret, info.AuthMode = cfg.APIKey, "api"
								}
							}
						}
					}
				}
			}

			// Otherwise fall back to an OAuth-based provider login (pi's /login).
			if info.AuthMode == "" {
				if data, err := os.ReadFile(authPath); err == nil {
					var auth map[string]json.RawMessage
					if json.Unmarshal(data, &auth) == nil && len(auth) > 0 {
						names := make([]string, 0, len(auth))
						for name := range auth {
							names = append(names, name)
						}
						sort.Strings(names)
						info.AuthMode = "oauth"
						info.Account = names[0]
					}
				}
			}

			return info.withDefaults("(provider default)"), nil
		},
	}
}
