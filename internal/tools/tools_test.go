package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if info.AuthMode != "chatgpt" {
		t.Errorf("authMode = %q, want chatgpt", info.AuthMode)
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
	var s map[string]any
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	_ = json.Unmarshal(data, &s)
	if s["theme"] != "dark" {
		t.Error("apply dropped existing 'theme' setting")
	}
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
	if _, hasKey := s.Env["ANTHROPIC_API_KEY"]; hasKey {
		t.Error("custom endpoint must not also set ANTHROPIC_API_KEY")
	}
	if s.Model != "some-model" {
		t.Errorf("model should be top-level, got %q", s.Model)
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
	// into opencode.json; auth.json must keep its existing login untouched.
	var cfg struct {
		Provider map[string]struct {
			Options struct {
				BaseURL string `json:"baseURL"`
				APIKey  string `json:"apiKey"`
			} `json:"options"`
			Models map[string]any `json:"models"`
		} `json:"provider"`
	}
	cfgData, _ := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	_ = json.Unmarshal(cfgData, &cfg)
	p, ok := cfg.Provider["charon"]
	if !ok {
		t.Fatal("provider 'charon' not written to opencode.json")
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

func TestNotDetectedInEmptyHome(t *testing.T) {
	sandboxHome(t)
	for _, name := range []string{"codex", "opencode"} {
		if Find(name).Detected() {
			t.Errorf("%s should not be detected in empty HOME", name)
		}
	}
}
