package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charon/internal/profile"
)

// sandbox points HOME and the store's XDG_CONFIG_HOME at temp dirs so run()
// never touches real user config (see AGENTS.md).
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USER", "tester")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

// seedCodex fakes an installed Codex CLI (auth.json makes it "detected").
func seedCodex(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "auth.json"), `{"auth_mode":"apikey","OPENAI_API_KEY":"sk-live"}`)
	writeTestFile(t, filepath.Join(dir, "config.toml"), "model = \"gpt-5\"\n")
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsUnknownCommandAndTool(t *testing.T) {
	sandbox(t)
	if err := run([]string{"bogus"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("run(bogus) = %v, want unknown-command error", err)
	}
	for _, args := range [][]string{
		{"rm", "faketool", "x"},
		{"switch", "faketool", "x"},
		{"undo", "faketool"},
		{"ls", "faketool"},
		{"cp", "faketool", "a", "b"},
	} {
		if err := run(args); err == nil || !strings.Contains(err.Error(), "unknown tool") {
			t.Errorf("run(%v) = %v, want unknown-tool error", args, err)
		}
	}
}

func TestRunProfileLifecycle(t *testing.T) {
	home := sandbox(t)
	seedCodex(t, home)

	if err := run([]string{"add", "codex", "--name", "work", "--key", "sk-test", "--endpoint", "https://example.com/v1"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := run([]string{"ls", "codex"}); err != nil {
		t.Fatalf("ls: %v", err)
	}
	if err := run([]string{"cp", "codex", "work", "work-2"}); err != nil {
		t.Fatalf("cp: %v", err)
	}
	if err := run([]string{"switch", "codex", "work-2"}); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if err := run([]string{"restore", "codex"}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if err := run([]string{"undo", "codex"}); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if err := run([]string{"rm", "codex", "work-2"}); err != nil {
		t.Fatalf("rm: %v", err)
	}

	store, err := profile.Open()
	if err != nil {
		t.Fatal(err)
	}
	if !store.Exists("codex", "work") || !store.Exists("codex", profile.DefaultName) {
		t.Errorf("expected work + default profiles, got %v", store.List("codex"))
	}
	if store.Exists("codex", "work-2") {
		t.Error("work-2 still exists after rm")
	}
}

func TestRunRejectsUnsafeProfileArguments(t *testing.T) {
	home := sandbox(t)
	seedCodex(t, home)

	for _, args := range [][]string{
		{"rm", "codex", "nope"},                             // nonexistent profile
		{"rm", "codex", "../.."},                            // traversal out of the store
		{"save", "codex", "../evil"},                        // credential snapshot outside the store
		{"add", "codex", "--name", "default", "--key", "k"}, // reserved name
		{"rename", "codex", "default", "x"},                 // default is not renamable
	} {
		if err := run(args); err == nil {
			t.Errorf("run(%v) succeeded, want error", args)
		}
	}
	// The traversal attempts must not have deleted the store or the config dir.
	if _, err := os.Stat(filepath.Join(home, ".config", "charon")); err != nil {
		t.Fatalf("store dir damaged: %v", err)
	}
}

func TestRunStatusAndVersion(t *testing.T) {
	home := sandbox(t)
	seedCodex(t, home)
	if err := run([]string{"status", "--json"}); err != nil {
		t.Errorf("status --json: %v", err)
	}
	if err := run([]string{"status"}); err != nil {
		t.Errorf("status: %v", err)
	}
	if err := run([]string{"version"}); err != nil {
		t.Errorf("version: %v", err)
	}
}
