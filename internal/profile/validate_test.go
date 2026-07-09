package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charon/internal/artifact"
	"charon/internal/tools"
)

// applyTool builds a tool with a single config file and an ApplyAuth that writes
// the spec into it, so AddProfile/EditProfile can be exercised end to end.
func applyTool(dir string) (*tools.Tool, string) {
	cfg := filepath.Join(dir, "config")
	return &tools.Tool{
		Name:      "applier",
		Title:     "Applier",
		Detected:  func() bool { _, err := os.Stat(cfg); return err == nil },
		Artifacts: []artifact.Artifact{artifact.NewFile("config", cfg, 0o600)},
		ApplyAuth: func(a tools.AuthSpec) error {
			return os.WriteFile(cfg, []byte(a.Endpoint+"|"+a.Key+"|"+a.Model), 0o600)
		},
	}, cfg
}

func TestSaveRejectsUnsafeNames(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c1")
	s := newStore(t)

	for _, name := range []string{"", ".", "..", "../evil", "a/b", "has space", DefaultName} {
		if err := s.Save(tool, name, "", ""); err == nil {
			t.Errorf("Save(%q) succeeded, want error", name)
		}
	}
	// Nothing may have been written outside (or inside) the profiles tree.
	if entries, err := os.ReadDir(filepath.Join(s.Root, "profiles")); err == nil && len(entries) != 0 {
		t.Errorf("unexpected profile dirs created: %v", entries)
	}
	if err := s.Save(tool, "user@example.com", "", ""); err != nil {
		t.Errorf("Save with valid name: %v", err)
	}
}

func TestAddProfileRejectsReservedNameBeforeApplying(t *testing.T) {
	dir := t.TempDir()
	tool, cfg := applyTool(dir)
	write(t, cfg, "pristine")
	s := newStore(t)

	for _, name := range []string{DefaultName, "../evil", ""} {
		if err := s.AddProfile(tool, name, Spec{Endpoint: "https://x", Key: "k"}); err == nil {
			t.Fatalf("AddProfile(%q) succeeded, want error", name)
		}
	}
	// The rejection must happen before ApplyAuth touches the live config.
	if got, _ := os.ReadFile(cfg); string(got) != "pristine" {
		t.Errorf("live config modified by rejected add: %q", got)
	}
}

func TestRemoveGuards(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c1")
	s := newStore(t)
	if err := s.Save(tool, "keep", "", ""); err != nil {
		t.Fatal(err)
	}

	if err := s.Remove(tool.Name, "nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("Remove(missing) = %v, want not-found error", err)
	}
	for _, name := range []string{"..", "../..", "a/b"} {
		if err := s.Remove(tool.Name, name); err == nil {
			t.Errorf("Remove(%q) succeeded, want error", name)
		}
	}
	// The store tree must have survived the traversal attempts.
	if !s.Exists(tool.Name, "keep") {
		t.Fatal("profile lost after rejected removals")
	}
}

func TestEditProfileRenames(t *testing.T) {
	dir := t.TempDir()
	tool, cfg := applyTool(dir)
	write(t, cfg, "seed")
	s := newStore(t)

	if err := s.AddProfile(tool, "old", Spec{Endpoint: "https://a", Key: "k1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.EditProfile(tool, "old", "new", Spec{Endpoint: "https://b", Key: "k2", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if s.Exists(tool.Name, "old") {
		t.Error("old profile still exists after rename-edit")
	}
	if !s.Exists(tool.Name, "new") {
		t.Fatal("new profile missing after rename-edit")
	}
	if sp, ok := s.GetSpec(tool.Name, "new"); !ok || sp.Endpoint != "https://b" || sp.Key != "k2" || sp.Model != "m" {
		t.Errorf("spec = %+v ok=%v, want the edited values", sp, ok)
	}
	if s.Active(tool.Name) != "new" {
		t.Errorf("active = %q, want new", s.Active(tool.Name))
	}
	if got, _ := os.ReadFile(cfg); string(got) != "https://b|k2|m" {
		t.Errorf("live config = %q, want the edited spec applied", got)
	}
}

func TestEditProfileSameNameKeepsProfile(t *testing.T) {
	dir := t.TempDir()
	tool, cfg := applyTool(dir)
	write(t, cfg, "seed")
	s := newStore(t)

	if err := s.AddProfile(tool, "work", Spec{Endpoint: "https://a", Key: "k1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.EditProfile(tool, "work", "work", Spec{Endpoint: "https://a", Key: "k2"}); err != nil {
		t.Fatal(err)
	}
	if !s.Exists(tool.Name, "work") {
		t.Fatal("profile missing after same-name edit")
	}
	if sp, _ := s.GetSpec(tool.Name, "work"); sp.Key != "k2" {
		t.Errorf("key = %q, want k2", sp.Key)
	}
}
