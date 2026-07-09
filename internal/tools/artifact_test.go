package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

func TestFileArtifactRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "secret.json")
	a := NewFile("secret.json", path, 0o600)

	if _, exists, err := a.Read(); err != nil || exists {
		t.Fatalf("expected absent artifact, exists=%v err=%v", exists, err)
	}

	if err := a.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, exists, err := a.Read()
	if err != nil || !exists || string(data) != "hello" {
		t.Fatalf("Read after write: data=%q exists=%v err=%v", data, exists, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %v, want 0600", info.Mode().Perm())
	}

	if err := a.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, exists, _ := a.Read(); exists {
		t.Error("artifact still exists after Remove")
	}
	// Removing a missing artifact is not an error.
	if err := a.Remove(); err != nil {
		t.Errorf("Remove(absent): %v", err)
	}
}

func TestMergedJSONFileMergePreservesNonOwnedKeys(t *testing.T) {
	a := NewMergedJSONFile("settings.json", filepath.Join(t.TempDir(), "settings.json"), 0o600, "env", "model")

	snapshot := []byte(`{"env":{"ANTHROPIC_API_KEY":"snap-key"},"model":"snap-model","theme":"dark"}`)
	live := []byte(`{"env":{"ANTHROPIC_API_KEY":"live-key"},"model":"live-model","theme":"light","effortLevel":"medium"}`)

	merged, err := a.Merge(snapshot, live)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "light" {
		t.Errorf("theme = %v, want live value %q (non-owned key must survive)", got["theme"], "light")
	}
	if got["effortLevel"] != "medium" {
		t.Errorf("effortLevel = %v, want live value %q (non-owned key, absent from snapshot)", got["effortLevel"], "medium")
	}
	if got["model"] != "snap-model" {
		t.Errorf("model = %v, want snapshot value %q (owned key must switch per profile)", got["model"], "snap-model")
	}
	env, _ := got["env"].(map[string]any)
	if env["ANTHROPIC_API_KEY"] != "snap-key" {
		t.Errorf("env.ANTHROPIC_API_KEY = %v, want snapshot value %q", env["ANTHROPIC_API_KEY"], "snap-key")
	}
}

func TestMergedJSONFileMergeDropsOwnedKeyAbsentFromSnapshot(t *testing.T) {
	a := NewMergedJSONFile("settings.json", filepath.Join(t.TempDir(), "settings.json"), 0o600, "model")

	snapshot := []byte(`{"theme":"dark"}`) // no "model" key in this profile
	live := []byte(`{"theme":"dark","model":"live-model"}`)

	merged, err := a.Merge(snapshot, live)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if _, exists := got["model"]; exists {
		t.Errorf("model = %v, want absent (snapshot has no owned value to restore)", got["model"])
	}
}

func TestMergedJSONFileMergeFallsBackWhenLiveEmptyOrUnparseable(t *testing.T) {
	a := NewMergedJSONFile("settings.json", filepath.Join(t.TempDir(), "settings.json"), 0o600, "model")
	snapshot := []byte(`{"model":"snap-model","theme":"dark"}`)

	if merged, err := a.Merge(snapshot, nil); err != nil || string(merged) != string(snapshot) {
		t.Errorf("Merge with empty live = %q, %v; want snapshot unchanged", merged, err)
	}
	if merged, err := a.Merge(snapshot, []byte("not json")); err != nil || string(merged) != string(snapshot) {
		t.Errorf("Merge with unparseable live = %q, %v; want snapshot unchanged", merged, err)
	}
}

func TestMergedTOMLFileMergePreservesNonOwnedKeys(t *testing.T) {
	a := NewMergedTOMLFile("config.toml", filepath.Join(t.TempDir(), "config.toml"), 0o600, "model")

	snapshot := []byte("model = \"snap-model\"\n")
	live := []byte("model = \"live-model\"\napproval_policy = \"never\"\n")

	merged, err := a.Merge(snapshot, live)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := toml.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "snap-model" {
		t.Errorf("model = %v, want snapshot value %q", got["model"], "snap-model")
	}
	if got["approval_policy"] != "never" {
		t.Errorf("approval_policy = %v, want live value %q (non-owned key must survive)", got["approval_policy"], "never")
	}
}

func TestMergedFileArtifactPeek(t *testing.T) {
	a := NewMergedJSONFile("settings.json", filepath.Join(t.TempDir(), "settings.json"), 0o600, "model", "effortLevel").
		WithDisplay("model", "effortLevel")

	model, effort := a.Peek([]byte(`{"model":"claude-haiku","effortLevel":"low","theme":"dark"}`))
	if model != "claude-haiku" {
		t.Errorf("model = %q, want claude-haiku", model)
	}
	if effort != "low" {
		t.Errorf("effort = %q, want low", effort)
	}

	// Missing keys, and a file with no display keys configured, both report "".
	if model, effort := a.Peek([]byte(`{"theme":"dark"}`)); model != "" || effort != "" {
		t.Errorf("Peek with no model/effort keys = %q, %q; want empty", model, effort)
	}
	noDisplay := NewMergedJSONFile("x", filepath.Join(t.TempDir(), "x"), 0o600, "model")
	if model, effort := noDisplay.Peek([]byte(`{"model":"m"}`)); model != "" || effort != "" {
		t.Errorf("Peek without WithDisplay = %q, %q; want empty", model, effort)
	}
	if model, effort := a.Peek([]byte("not json")); model != "" || effort != "" {
		t.Errorf("Peek on unparseable data = %q, %q; want empty", model, effort)
	}
}

func TestAtomicWriteReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := atomicWrite(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "v2" {
		t.Errorf("got %q, want v2", data)
	}
}
