package tools

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

// sandboxHome points HOME (and USER) at a temp dir so tool paths resolve there
// and no real user config is touched.
func sandboxHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USER", "tester")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFindUnknown(t *testing.T) {
	if Find("nope") != nil {
		t.Error("expected nil for unknown tool")
	}
}

func TestCodexDescribeAndApply(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "model = \"gpt-5.5\"\n")
	writeFile(t, filepath.Join(home, ".codex", "auth.json"), `{"auth_mode":"chatgpt","tokens":{"access_token":"a"}}`)

	c := Find("codex")
	if !c.Detected() {
		t.Fatal("codex should be detected")
	}
	info, _ := c.Describe()
	// A ChatGPT OAuth login (auth_mode "chatgpt" on disk) surfaces as "oauth".
	if info.AuthMode != "oauth" {
		t.Errorf("authMode = %q, want oauth", info.AuthMode)
	}

	err := c.ApplyAuth(AuthSpec{Endpoint: "https://proxy/v1", Key: "sk-k123456789", Model: "gpt-5.5"})
	if err != nil {
		t.Fatal(err)
	}
	info, _ = c.Describe()
	if info.Endpoint != "https://proxy/v1" || info.AuthMode != "api" || info.Secret != "sk-k123456789" {
		t.Errorf("after apply: %#v", info)
	}
	// The key must be embedded inline in config.toml, and the ChatGPT OAuth
	// tokens in auth.json must be left untouched.
	cfgData, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if !strings.Contains(string(cfgData), "experimental_bearer_token") {
		t.Error("config.toml missing inline bearer token")
	}
	var auth map[string]any
	data, _ := os.ReadFile(filepath.Join(home, ".codex", "auth.json"))
	_ = json.Unmarshal(data, &auth)
	if auth["auth_mode"] != "chatgpt" || auth["tokens"] == nil {
		t.Error("apply must not modify auth.json (ChatGPT OAuth)")
	}
}

func TestCodexPinsClaudeContextWindow(t *testing.T) {
	home := sandboxHome(t)
	// A prior Claude profile pinned the window; switching to an OpenAI model
	// (which Codex already sizes from its catalog) must clear it.
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), "model_context_window = 200000\n")

	c := Find("codex")
	window := func() (int64, bool) {
		var cfg struct {
			Window *int64 `toml:"model_context_window"`
		}
		data, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
		_ = toml.Unmarshal(data, &cfg)
		if cfg.Window == nil {
			return 0, false
		}
		return *cfg.Window, true
	}

	// A Claude model routed through the custom provider gets its window pinned
	// so Codex stops overrunning it with the 272K fallback.
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gw/v1", Key: "sk-k123456789", Model: "claude-opus-4-7"}); err != nil {
		t.Fatal(err)
	}
	if w, ok := window(); !ok || w != 200000 {
		t.Errorf("claude model should pin model_context_window=200000, got %d (set=%v)", w, ok)
	}

	// Switching to a model Codex knows must drop the stale pin.
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gw/v1", Key: "sk-k123456789", Model: "gpt-5.5"}); err != nil {
		t.Fatal(err)
	}
	if w, ok := window(); ok {
		t.Errorf("non-claude model must clear model_context_window, got %d", w)
	}
}

func TestCodexKeepsUserProvider(t *testing.T) {
	home := sandboxHome(t)
	// The user has their own custom Codex provider already configured.
	writeFile(t, filepath.Join(home, ".codex", "config.toml"),
		"[model_providers.myllm]\nname = \"mine\"\nbase_url = \"https://mine/v1\"\n")

	c := Find("codex")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gw/v1", Key: "sk-k123456789", Model: "gpt-x"}); err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		ModelProviders map[string]struct {
			Name    string `toml:"name"`
			BaseURL string `toml:"base_url"`
		} `toml:"model_providers"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if p, ok := cfg.ModelProviders["myllm"]; !ok || p.BaseURL != "https://mine/v1" {
		t.Errorf("charon altered or removed the user's provider 'myllm': %#v", cfg.ModelProviders["myllm"])
	}
	if _, ok := cfg.ModelProviders["charon"]; !ok {
		t.Error("charon provider not added")
	}
}

func TestClaudeDescribeAndApply(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"theme":"dark"}`)

	c := Find("claude")
	if !c.Detected() {
		t.Fatal("claude should be detected via settings.json")
	}
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://api.anthropic.com", Key: "sk-ant-123456789", Model: "claude-opus-4-8"}); err != nil {
		t.Fatal(err)
	}
	info, _ := c.Describe()
	if info.AuthMode != "api" || info.Model != "claude-opus-4-8" {
		t.Errorf("after apply: %#v", info)
	}
	var s struct {
		Theme string            `json:"theme"`
		Env   map[string]string `json:"env"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)
	if s.Theme != "dark" {
		t.Error("apply dropped existing 'theme' setting")
	}
	if s.Env["ANTHROPIC_API_KEY"] != "sk-ant-123456789" {
		t.Errorf("stock endpoint should set ANTHROPIC_API_KEY, got env=%v", s.Env)
	}
	// The stock Anthropic endpoint must NOT set ANTHROPIC_BASE_URL — doing so
	// makes Claude Code treat it as a third-party gateway and disable connectors.
	if _, ok := s.Env["ANTHROPIC_BASE_URL"]; ok {
		t.Errorf("stock endpoint must not set ANTHROPIC_BASE_URL, got env=%v", s.Env)
	}
}

func TestClaudeApplyClearsStaleBaseURL(t *testing.T) {
	home := sandboxHome(t)
	// A previously-applied custom-gateway profile left a base URL behind.
	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"env":{"ANTHROPIC_BASE_URL":"https://gateway.example/v1","ANTHROPIC_AUTH_TOKEN":"sk-gw-old"}}`)

	c := Find("claude")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://api.anthropic.com", Key: "sk-ant-123456789"}); err != nil {
		t.Fatal(err)
	}
	var s struct {
		Env map[string]string `json:"env"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)
	if _, ok := s.Env["ANTHROPIC_BASE_URL"]; ok {
		t.Errorf("switching to stock endpoint must clear stale ANTHROPIC_BASE_URL, got env=%v", s.Env)
	}
	if _, ok := s.Env["ANTHROPIC_AUTH_TOKEN"]; ok {
		t.Errorf("switching to stock endpoint must clear stale ANTHROPIC_AUTH_TOKEN, got env=%v", s.Env)
	}
}

func TestClaudeApplyApprovesAPIKey(t *testing.T) {
	home := sandboxHome(t)
	key := "sk-ant-abcdefghij0123456789" // last claudeKeyIDLen chars => "hij0123456789" padded
	id := key[len(key)-claudeKeyIDLen:]
	// Simulate a prior "No" that left this key disabled in Claude Code.
	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"customApiKeyResponses":{"approved":[],"disabled":["`+id+`"]}}`)

	c := Find("claude")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://api.anthropic.com", Key: key}); err != nil {
		t.Fatal(err)
	}

	var s struct {
		Resp struct {
			Approved []string `json:"approved"`
			Disabled []string `json:"disabled"`
		} `json:"customApiKeyResponses"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)

	if !contains(s.Resp.Approved, id) {
		t.Errorf("key id %q should be approved, got %v", id, s.Resp.Approved)
	}
	if contains(s.Resp.Disabled, id) {
		t.Errorf("key id %q must be removed from disabled, got %v", id, s.Resp.Disabled)
	}
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

func TestClaudeCustomEndpointUsesBearer(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{}`)

	c := Find("claude")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gateway.example/v1", Key: "sk-gw-123456789", Model: "some-model"}); err != nil {
		t.Fatal(err)
	}
	var s struct {
		Model string            `json:"model"`
		Env   map[string]string `json:"env"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)
	if s.Env["ANTHROPIC_AUTH_TOKEN"] != "sk-gw-123456789" {
		t.Errorf("custom endpoint should use ANTHROPIC_AUTH_TOKEN, got env=%v", s.Env)
	}
	// Claude Code appends "/v1/messages" to the base URL, so a trailing "/v1"
	// (common in OpenAI-style gateway docs) is stripped to avoid "/v1/v1".
	if s.Env["ANTHROPIC_BASE_URL"] != "https://gateway.example" {
		t.Errorf("custom endpoint should set normalized ANTHROPIC_BASE_URL, got env=%v", s.Env)
	}
	if _, hasKey := s.Env["ANTHROPIC_API_KEY"]; hasKey {
		t.Error("custom endpoint must not also set ANTHROPIC_API_KEY")
	}
	if bytes.Contains(data, []byte("customApiKeyResponses")) {
		t.Error("bearer-token endpoint must not touch customApiKeyResponses")
	}
	// A custom gateway's model must go through ANTHROPIC_MODEL, not the
	// top-level "model" selector (which Claude Code validates against its
	// built-in catalog and rejects for unknown gateway models).
	if s.Env["ANTHROPIC_MODEL"] != "some-model" {
		t.Errorf("custom endpoint model should be in ANTHROPIC_MODEL, got env=%v", s.Env)
	}
	if s.Model != "" {
		t.Errorf("custom endpoint must not set top-level model, got %q", s.Model)
	}
}

func TestNormalizeClaudeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://api.reii.site/v1":  "https://api.reii.site",
		"https://api.reii.site/v1/": "https://api.reii.site",
		"https://api.reii.site":     "https://api.reii.site",
		"https://api.reii.site/":    "https://api.reii.site",
		"https://gw.example/v1beta": "https://gw.example/v1beta",
	}
	for in, want := range cases {
		if got := normalizeClaudeBaseURL(in); got != want {
			t.Errorf("normalizeClaudeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClaudeApplyClearsStaleTopLevelModel(t *testing.T) {
	home := sandboxHome(t)
	// A prior default-endpoint session left a top-level "opus" alias behind.
	// On a custom gateway that alias resolves to a model the gateway can't
	// serve, producing "model may not exist / no access".
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"opus"}`)

	c := Find("claude")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gateway.example/v1", Key: "sk-gw-123456789", Model: "gateway-model"}); err != nil {
		t.Fatal(err)
	}
	var s struct {
		Model string            `json:"model"`
		Env   map[string]string `json:"env"`
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)
	if s.Model != "" {
		t.Errorf("switching to custom endpoint must clear stale top-level model, got %q", s.Model)
	}
	if s.Env["ANTHROPIC_MODEL"] != "gateway-model" {
		t.Errorf("custom endpoint model should be in ANTHROPIC_MODEL, got env=%v", s.Env)
	}
}

func TestOpenCodeDescribeAndApply(t *testing.T) {
	home := sandboxHome(t)
	authPath := filepath.Join(home, ".local", "share", "opencode", "auth.json")
	writeFile(t, authPath, `{"opencode":{"type":"api","key":"sk-existing"}}`)

	c := Find("opencode")
	if !c.Detected() {
		t.Fatal("opencode should be detected via auth.json")
	}
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://openrouter.ai/api/v1", Key: "sk-or-123456789", Model: "x/y"}); err != nil {
		t.Fatal(err)
	}
	// The provider (with key in options.apiKey and a models map) must be written
	// into the config. With no existing config file, charon defaults to the
	// current opencode.jsonc name; auth.json must keep its existing login.
	var cfg struct {
		Provider map[string]struct {
			Options struct {
				BaseURL string `json:"baseURL"`
				APIKey  string `json:"apiKey"`
			} `json:"options"`
			Models map[string]any `json:"models"`
		} `json:"provider"`
	}
	cfgData, _ := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.jsonc"))
	_ = json.Unmarshal(cfgData, &cfg)
	p, ok := cfg.Provider["charon"]
	if !ok {
		t.Fatal("provider 'charon' not written to opencode.jsonc")
	}
	if p.Options.APIKey != "sk-or-123456789" {
		t.Errorf("apiKey not in options: %#v", p.Options)
	}
	if len(p.Models) == 0 {
		t.Error("models map is empty; models won't appear in opencode")
	}
	var auth map[string]any
	data, _ := os.ReadFile(authPath)
	_ = json.Unmarshal(data, &auth)
	if auth["opencode"] == nil {
		t.Error("apply must not drop existing login 'opencode' in auth.json")
	}
}

func TestOpenCodeEditsExistingJsoncInPlace(t *testing.T) {
	home := sandboxHome(t)
	dir := filepath.Join(home, ".config", "opencode")
	jsonc := filepath.Join(dir, "opencode.jsonc")
	// The user already has their own provider configured in opencode.jsonc.
	writeFile(t, jsonc, `{"$schema":"https://opencode.ai/config.json","provider":{"myllm":{"name":"mine"}}}`)

	c := Find("opencode")
	if err := c.ApplyAuth(AuthSpec{Endpoint: "https://gw/v1", Key: "sk-abc", Model: "gpt-x"}); err != nil {
		t.Fatal(err)
	}

	// charon must edit the existing opencode.jsonc, not write a second
	// opencode.json that opencode would ignore.
	if _, err := os.Stat(filepath.Join(dir, "opencode.json")); err == nil {
		t.Error("charon wrote a stray opencode.json instead of editing opencode.jsonc")
	}

	var cfg struct {
		Provider map[string]json.RawMessage `json:"provider"`
	}
	data, _ := os.ReadFile(jsonc)
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	// The user's original provider must survive untouched alongside charon's.
	if _, ok := cfg.Provider["myllm"]; !ok {
		t.Error("charon removed the user's original provider 'myllm'")
	}
	if _, ok := cfg.Provider["charon"]; !ok {
		t.Error("charon provider not added to opencode.jsonc")
	}
}

func TestEnsureOnlyCharonChanged(t *testing.T) {
	provider := map[string]any{
		"myllm":  map[string]any{"name": "mine"},
		"charon": map[string]any{"name": "charon"},
	}
	original := snapshotProviders(provider) // captures myllm only

	// Updating only charon is allowed.
	provider["charon"] = map[string]any{"name": "charon", "options": map[string]any{"baseURL": "x"}}
	if err := ensureOnlyCharonChanged(original, provider); err != nil {
		t.Errorf("changing only charon must be allowed, got %v", err)
	}

	// Editing the user's provider is refused.
	edited := map[string]any{"myllm": map[string]any{"name": "hijacked"}}
	if err := ensureOnlyCharonChanged(original, edited); err == nil {
		t.Error("editing an original provider must be refused")
	}

	// Deleting the user's provider is refused.
	deleted := map[string]any{"charon": map[string]any{"name": "charon"}}
	if err := ensureOnlyCharonChanged(original, deleted); err == nil {
		t.Error("deleting an original provider must be refused")
	}
}

func TestNotDetectedInEmptyHome(t *testing.T) {
	sandboxHome(t)
	for _, name := range []string{"codex", "opencode"} {
		if Find(name).Detected() {
			t.Errorf("%s should not be detected in empty HOME", name)
		}
	}
}
