package tools

import (
	"path/filepath"
	"testing"
)

func TestCodexDescribeAPIKeyAuthMode(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".codex", "auth.json"),
		`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-plain-123"}`)

	info, _ := Find("codex").Describe()
	if info.AuthMode != "api" || info.Secret != "sk-plain-123" {
		t.Errorf("apikey auth_mode: got AuthMode=%q Secret=%q, want api/sk-plain-123", info.AuthMode, info.Secret)
	}
}

func TestCodexDescribeUnknownAuthModePassesThrough(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".codex", "auth.json"), `{"auth_mode":"something-else"}`)

	info, _ := Find("codex").Describe()
	if info.AuthMode != "something-else" {
		t.Errorf("AuthMode = %q, want passthrough of unrecognized auth_mode", info.AuthMode)
	}
}

func TestClaudeContextWindow(t *testing.T) {
	cases := map[string]int{
		"claude-opus-4-7": 200_000,
		"CLAUDE-SONNET":   200_000,
		"gpt-5.5":         0,
		"":                0,
	}
	for model, want := range cases {
		if got := claudeContextWindow(model); got != want {
			t.Errorf("claudeContextWindow(%q) = %d, want %d", model, got, want)
		}
	}
}

func TestClaudeKeyIDShortKeyReturnsAsIs(t *testing.T) {
	if got := claudeKeyID("short"); got != "short" {
		t.Errorf("claudeKeyID(short) = %q, want unchanged", got)
	}
}

func TestOpenCodeDescribeFallsBackToAuthJSONLogin(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".local", "share", "opencode", "auth.json"),
		`{"anthropic":{"type":"oauth"}}`)

	info, _ := Find("opencode").Describe()
	if info.AuthMode != "oauth (anthropic)" {
		t.Errorf("AuthMode = %q, want oauth (anthropic)", info.AuthMode)
	}
}

func TestOpenCodeDescribeFallsBackToNonCharonProviderEndpoint(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".config", "opencode", "opencode.jsonc"),
		`{"provider":{"myllm":{"options":{"baseURL":"https://mine/v1"}}}}`)

	info, _ := Find("opencode").Describe()
	if info.Endpoint != "https://mine/v1" {
		t.Errorf("Endpoint = %q, want fallback to user provider's baseURL", info.Endpoint)
	}
}

func TestOpenCodeConfigPathPrefersExistingJSONC(t *testing.T) {
	home := sandboxHome(t)
	// No config yet: charon must default to the jsonc path.
	if got, want := opencodeConfigPath(), filepath.Join(home, ".config", "opencode", "opencode.jsonc"); got != want {
		t.Errorf("default config path = %q, want %q", got, want)
	}

	// A legacy opencode.json on disk (and no .jsonc) must be edited in place.
	writeFile(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{}`)
	if got, want := opencodeConfigPath(), filepath.Join(home, ".config", "opencode", "opencode.json"); got != want {
		t.Errorf("legacy config path = %q, want %q", got, want)
	}
}
