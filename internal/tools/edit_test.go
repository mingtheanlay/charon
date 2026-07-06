package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONMapRoundTripPreservesKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := loadJSONMap(path)
	if err != nil {
		t.Fatal(err)
	}
	env := subMap(m, "env")
	env["ANTHROPIC_API_KEY"] = "sk-x"
	if err := writeJSONMap(path, m, 0o600); err != nil {
		t.Fatal(err)
	}

	got, _ := loadJSONMap(path)
	if got["theme"] != "dark" {
		t.Error("existing key 'theme' was dropped")
	}
	gotEnv, ok := got["env"].(map[string]any)
	if !ok || gotEnv["ANTHROPIC_API_KEY"] != "sk-x" {
		t.Errorf("nested key not written: %#v", got["env"])
	}
}

func TestLoadJSONMapAbsentAndEmpty(t *testing.T) {
	dir := t.TempDir()
	if m, err := loadJSONMap(filepath.Join(dir, "nope.json")); err != nil || len(m) != 0 {
		t.Errorf("absent: m=%v err=%v", m, err)
	}
	empty := filepath.Join(dir, "empty.json")
	_ = os.WriteFile(empty, nil, 0o600)
	if m, err := loadJSONMap(empty); err != nil || len(m) != 0 {
		t.Errorf("empty: m=%v err=%v", m, err)
	}
}

func TestTOMLMapRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	_ = os.WriteFile(path, []byte("model = \"gpt-5.5\"\n[features]\njs_repl = false\n"), 0o644)

	m, err := loadTOMLMap(path)
	if err != nil {
		t.Fatal(err)
	}
	m["model_provider"] = "custom"
	providers := subMap(m, "model_providers")
	providers["custom"] = map[string]any{"base_url": "http://x"}
	if err := writeTOMLMap(path, m, 0o644); err != nil {
		t.Fatal(err)
	}

	got, _ := loadTOMLMap(path)
	if got["model"] != "gpt-5.5" || got["model_provider"] != "custom" {
		t.Errorf("round trip lost data: %#v", got)
	}
	feat, _ := got["features"].(map[string]any)
	if feat == nil || feat["js_repl"] != false {
		t.Errorf("nested table lost: %#v", got["features"])
	}
}

func TestSubMapCreatesAndReuses(t *testing.T) {
	m := map[string]any{}
	a := subMap(m, "k")
	a["x"] = 1
	b := subMap(m, "k")
	if b["x"] != 1 {
		t.Error("subMap did not reuse existing map")
	}
}
