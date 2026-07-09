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
	// An oauth login carries no email, unlike Claude/Codex — the provider name is the
	// account identity, so "back up this login" can name the profile automatically
	// instead of failing with "no logged-in account detected".
	if info.Account != "anthropic" {
		t.Errorf("Account = %q, want anthropic (provider-name fallback for oauth logins)", info.Account)
	}
}

// TestOpenCodeSaveCurrentAccountAfterProviderLogin reproduces "sign into a provider
// for the default profile, then can't back it up": SaveCurrentAccount requires
// Describe().Account to be non-empty, which used to never happen for OpenCode.
func TestOpenCodeSaveCurrentAccountAfterProviderLogin(t *testing.T) {
	sandboxHome(t)
	writeFile(t, filepath.Join(home(), ".local", "share", "opencode", "auth.json"),
		`{"github-copilot":{"type":"oauth","refresh":"r","access":"a","expires":0}}`)

	info, err := Find("opencode").Describe()
	if err != nil {
		t.Fatal(err)
	}
	if info.Account == "" {
		t.Fatal("Account should be set after an oauth provider login, so backup can name the profile")
	}
	if info.Account != "github-copilot" {
		t.Errorf("Account = %q, want github-copilot", info.Account)
	}
}

// TestOpenCodeAccountDetectedDespiteUnrelatedAPIKeyEntry reproduces the real bug: a
// user connects to ChatGPT via OpenCode's own `/connect`, but auth.json also has an
// unrelated "opencode" api-key entry (OpenCode's own hosted-models login) — which
// used to win AuthMode first and suppress Account detection entirely, so "back up
// this login" never found an account to name the profile after.
func TestOpenCodeAccountDetectedDespiteUnrelatedAPIKeyEntry(t *testing.T) {
	sandboxHome(t)
	// Matches the real shape of an OpenAI access token (as ChatGPT via /connect stores
	// it): email nested under this OIDC profile claim, not a top-level "email".
	jwt := makeJWT(t, map[string]any{
		"https://api.openai.com/profile": map[string]any{"email": "user@example.com"},
	})
	writeFile(t, filepath.Join(home(), ".local", "share", "opencode", "auth.json"), `{
		"opencode": {"type":"api","key":"sk-opencode-own-key"},
		"openai": {"type":"oauth","access":"`+jwt+`","refresh":"r","expires":0}
	}`)

	info, err := Find("opencode").Describe()
	if err != nil {
		t.Fatal(err)
	}
	if info.Account != "user@example.com" {
		t.Errorf("Account = %q, want user@example.com (the ChatGPT oauth login's JWT email)", info.Account)
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
