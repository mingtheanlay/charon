package profile

import (
	"os"
	"path/filepath"
	"testing"

	"charon/internal/tools"
)

// fakeTool builds a tool whose auth surface is two files under dir, so the
// store can be exercised without touching real configs.
func fakeTool(dir string) (*tools.Tool, string, string) {
	cfg := filepath.Join(dir, "config")
	auth := filepath.Join(dir, "auth")
	return &tools.Tool{
		Name:     "fake",
		Title:    "Fake",
		Detected: func() bool { _, err := os.Stat(cfg); return err == nil },
		Artifacts: []tools.Artifact{
			tools.NewFile("config", cfg, 0o644),
			tools.NewFile("auth", auth, 0o600),
		},
	}, cfg, auth
}

// newStore roots the store in an isolated temp dir.
func newStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureOriginalAndActive(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, auth := fakeTool(dir)
	write(t, cfg, "c1")
	write(t, auth, "a1")

	s := newStore(t)
	if err := s.EnsureOriginal(tool); err != nil {
		t.Fatal(err)
	}
	if s.Active("fake") != OriginalName {
		t.Errorf("active = %q, want original", s.Active("fake"))
	}
	// Calling again must not overwrite the captured original.
	write(t, cfg, "changed")
	if err := s.EnsureOriginal(tool); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(tool, OriginalName); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(cfg); string(got) != "c1" {
		t.Errorf("original not preserved: got %q", got)
	}
}

func TestSaveSwitchRoundTrip(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, auth := fakeTool(dir)
	write(t, cfg, "orig-cfg")
	write(t, auth, "orig-auth")

	s := newStore(t)
	if err := s.EnsureOriginal(tool); err != nil {
		t.Fatal(err)
	}

	// Change live config and save as "work".
	write(t, cfg, "work-cfg")
	write(t, auth, "work-auth")
	if err := s.Save(tool, "work", "Work", ""); err != nil {
		t.Fatal(err)
	}

	names := s.List("fake")
	if len(names) != 2 || names[0] != OriginalName {
		t.Errorf("List = %v, want [original work]", names)
	}

	// Switch back to original.
	if _, err := s.Apply(tool, OriginalName); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(cfg); string(got) != "orig-cfg" {
		t.Errorf("cfg after restore = %q", got)
	}

	// Switch to work again.
	if _, err := s.Apply(tool, "work"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(auth); string(got) != "work-auth" {
		t.Errorf("auth after switch = %q", got)
	}
	if s.Active("fake") != "work" {
		t.Errorf("active = %q, want work", s.Active("fake"))
	}
}

func TestApplyMakesBackup(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, auth := fakeTool(dir)
	write(t, cfg, "c")
	write(t, auth, "a")

	s := newStore(t)
	_ = s.EnsureOriginal(tool)
	write(t, cfg, "current")
	backup, err := s.Apply(tool, OriginalName)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(backup, "config")); string(got) != "current" {
		t.Errorf("backup did not capture pre-switch state: %q", got)
	}
}

func TestApplyRemovesAbsentArtifact(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, auth := fakeTool(dir)
	write(t, cfg, "c")
	// No auth file exists — snapshot should record it absent.

	s := newStore(t)
	if err := s.Save(tool, "noauth", "", ""); err != nil {
		t.Fatal(err)
	}

	// Now a live auth file appears; applying "noauth" must remove it.
	write(t, auth, "leaked")
	if _, err := s.Apply(tool, "noauth"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(auth); !os.IsNotExist(err) {
		t.Error("apply did not remove artifact absent from the profile")
	}
}

func TestSaveWithSpecAndGetSpec(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")

	s := newStore(t)
	want := Spec{Endpoint: "https://x/v1", Key: "sk-123", Model: "m1"}
	if err := s.SaveWithSpec(tool, "p", want); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetSpec("fake", "p")
	if !ok || got != want {
		t.Errorf("GetSpec = %+v, ok=%v; want %+v", got, ok, want)
	}
	// A plain Save records no spec.
	_ = s.Save(tool, "plain", "", "")
	if _, ok := s.GetSpec("fake", "plain"); ok {
		t.Error("plain Save should not record a spec")
	}
}

func TestRemoveProfile(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")

	s := newStore(t)
	_ = s.Save(tool, "temp", "", "")
	if !s.Exists("fake", "temp") {
		t.Fatal("profile not saved")
	}
	if err := s.Remove("fake", "temp"); err != nil {
		t.Fatal(err)
	}
	if s.Exists("fake", "temp") {
		t.Error("profile still exists after Remove")
	}
	if err := s.Remove("fake", OriginalName); err == nil {
		t.Error("removing original should fail")
	}
}
