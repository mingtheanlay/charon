package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	if info.Endpoint != "https://proxy/v1" || info.AuthMode != "apikey" || info.Secret != "sk-k123456789" {
		t.Errorf("after apply: %#v", info)
	}
	// Existing tokens must be preserved in auth.json.
	var auth map[string]any
	data, _ := os.ReadFile(filepath.Join(home, ".codex", "auth.json"))
	_ = json.Unmarshal(data, &auth)
	if auth["tokens"] == nil {
		t.Error("apply dropped existing tokens field")
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
	var auth map[string]any
	data, _ := os.ReadFile(authPath)
	_ = json.Unmarshal(data, &auth)
	if auth["opencode"] == nil {
		t.Error("apply dropped existing provider 'opencode'")
	}
	if auth["aies"] == nil {
		t.Error("apply did not add provider 'aies'")
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
