package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"charon/internal/artifact"
	"charon/internal/secret"
)

const claudeKeychainService = "Claude Code-credentials"

// newClaude describes Claude Code: API keys in ~/.claude/settings.json, OAuth in the keychain.
func newClaude() *Tool {
	settingsPath := filepath.Join(home(), ".claude", "settings.json")

	return &Tool{
		Name:            "claude",
		Title:           "Claude Code",
		Provider:        "anthropic",
		DefaultEndpoint: "https://api.anthropic.com",
		Artifacts: []artifact.Artifact{
			// theme is a display preference, not per-profile auth — preserved live. model and
			// effortLevel switch with the profile, so each account remembers its own choice.
			artifact.NewMergedJSONFile("settings.json", settingsPath, 0o600, "env", "customApiKeyResponses", "model", "effortLevel").
				WithDisplay("model", "effortLevel"),
			artifact.NewKeychain("credentials", claudeKeychainService, os.Getenv("USER")),
		},
		ApplyAuth: func(a AuthSpec) error {
			s, err := loadJSONMap(settingsPath)
			if err != nil {
				return err
			}
			env := subMap(s, "env")
			// Clear every auth key so we never send conflicting headers or a stale base URL.
			delete(env, "ANTHROPIC_API_KEY")
			delete(env, "ANTHROPIC_AUTH_TOKEN")
			delete(env, "ANTHROPIC_BASE_URL")
			delete(env, "ANTHROPIC_MODEL")

			custom := a.Endpoint != "" && !strings.Contains(a.Endpoint, "api.anthropic.com")
			if custom {
				// Gateways want Bearer auth at a custom base URL.
				env["ANTHROPIC_BASE_URL"] = normalizeClaudeBaseURL(a.Endpoint)
				env["ANTHROPIC_AUTH_TOKEN"] = a.Key
				// Gateway models aren't in Claude Code's catalog; the top-level "model"
				// selector validates against it and rejects them, so route via ANTHROPIC_MODEL.
				delete(s, "model")
				if a.Model != "" {
					env["ANTHROPIC_MODEL"] = a.Model
				}
			} else {
				// Anthropic's own API uses x-api-key. Leave ANTHROPIC_BASE_URL unset: pointing it
				// at the default endpoint makes Claude Code treat it as a gateway and break connectors.
				env["ANTHROPIC_API_KEY"] = a.Key
				// Pre-approve the key (and un-disable it) so a prior "No" can't leave it ignored.
				approveClaudeAPIKey(s, a.Key)
				// Stock models are in the catalog, so the top-level selector is preferred.
				if a.Model != "" {
					s["model"] = a.Model
				}
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
					Model       string `json:"model"`
					EffortLevel string `json:"effortLevel"`
					Env         struct {
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
					info.Effort = s.EffortLevel
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

			info.Account = claudeAccountEmail()

			return info.withDefaults("api.anthropic.com (default)"), nil
		},
	}
}

// claudeAccountEmail reads the logged-in account's email from ~/.claude.json for
// display/naming only — the file is never written or snapshotted. "" if absent.
func claudeAccountEmail() string {
	data, err := os.ReadFile(filepath.Join(home(), ".claude.json"))
	if err != nil {
		return ""
	}
	var c struct {
		OAuthAccount struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"oauthAccount"`
	}
	if json.Unmarshal(data, &c) != nil {
		return ""
	}
	return c.OAuthAccount.EmailAddress
}

// normalizeClaudeBaseURL trims a trailing "/v1": Claude appends "/v1/messages", so a
// "/v1" base URL 404s as "/v1/v1/messages". (Claude-only; Codex genuinely wants "/v1".)
func normalizeClaudeBaseURL(ep string) string {
	ep = strings.TrimRight(ep, "/")
	ep = strings.TrimSuffix(ep, "/v1")
	return strings.TrimRight(ep, "/")
}

// claudeKeyIDLen is how many trailing characters of a key Claude Code uses as its ID.
const claudeKeyIDLen = 20

// claudeKeyID is how Claude Code identifies a key: its last claudeKeyIDLen characters.
func claudeKeyID(key string) string {
	if len(key) <= claudeKeyIDLen {
		return key
	}
	return key[len(key)-claudeKeyIDLen:]
}

// approveClaudeAPIKey marks key approved and un-disabled in customApiKeyResponses,
// so Claude Code stops prompting and a prior rejection can't keep it switched off.
func approveClaudeAPIKey(s map[string]any, key string) {
	if key == "" {
		return
	}
	id := claudeKeyID(key)
	resp := subMap(s, "customApiKeyResponses")

	resp["approved"] = addString(toStringSlice(resp["approved"]), id)
	resp["disabled"] = removeString(toStringSlice(resp["disabled"]), id)
}

// toStringSlice coerces a JSON-decoded []any (or []string) into a []string.
func toStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func addString(list []string, s string) []string {
	for _, e := range list {
		if e == s {
			return list
		}
	}
	return append(list, s)
}

func removeString(list []string, s string) []string {
	out := list[:0]
	for _, e := range list {
		if e != s {
			out = append(out, e)
		}
	}
	return out
}
