package profile

import (
	"encoding/json"
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

// fakeRotator is an in-memory artifact that reports Rotates()==true, standing in for
// a keychain entry (e.g. Claude Code's OAuth token) without touching the real OS
// keychain, which tests must never do.
type fakeRotator struct {
	id    string
	value *string // nil = absent
}

func (f *fakeRotator) ID() string    { return f.id }
func (f *fakeRotator) Rotates() bool { return true }
func (f *fakeRotator) Read() ([]byte, bool, error) {
	if f.value == nil {
		return nil, false, nil
	}
	return []byte(*f.value), true, nil
}
func (f *fakeRotator) Write(data []byte) error {
	s := string(data)
	f.value = &s
	return nil
}
func (f *fakeRotator) Remove() error {
	f.value = nil
	return nil
}

// rotatingTool builds a tool with a plain config file plus a fakeRotator standing in
// for a keychain-backed credential, so the store can be exercised without the OS
// keychain.
func rotatingTool(dir string) (*tools.Tool, string, *fakeRotator) {
	cfg := filepath.Join(dir, "config")
	rot := &fakeRotator{id: "credentials"}
	return &tools.Tool{
		Name:      "rotating",
		Title:     "Rotating",
		Detected:  func() bool { _, err := os.Stat(cfg); return err == nil },
		Artifacts: []tools.Artifact{tools.NewFile("config", cfg, 0o644), rot},
	}, cfg, rot
}

// mergedTool builds a tool whose config is a JSON file mixing a "model" (profile-
// owned) key with a "theme" (live CLI preference) key, so switching profiles can be
// exercised without touching a real Claude/Codex/OpenCode config.
func mergedTool(dir string) (*tools.Tool, string) {
	cfg := filepath.Join(dir, "settings.json")
	return &tools.Tool{
		Name:     "merged",
		Title:    "Merged",
		Detected: func() bool { _, err := os.Stat(cfg); return err == nil },
		Artifacts: []tools.Artifact{
			tools.NewMergedJSONFile("settings.json", cfg, 0o600, "model"),
		},
	}, cfg
}

// mergedToolWithDisplay is mergedTool plus an effortLevel owned key and Peek support
// for both, so ProfileModelEffort can be exercised end to end.
func mergedToolWithDisplay(dir string) (*tools.Tool, string) {
	cfg := filepath.Join(dir, "settings.json")
	return &tools.Tool{
		Name:     "merged-display",
		Title:    "Merged Display",
		Detected: func() bool { _, err := os.Stat(cfg); return err == nil },
		Artifacts: []tools.Artifact{
			tools.NewMergedJSONFile("settings.json", cfg, 0o600, "model", "effortLevel").
				WithDisplay("model", "effortLevel"),
		},
	}, cfg
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

func TestEnsureDefaultAndActive(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, auth := fakeTool(dir)
	write(t, cfg, "c1")
	write(t, auth, "a1")

	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}
	if s.Active("fake") != DefaultName {
		t.Errorf("active = %q, want default", s.Active("fake"))
	}
	// Calling again must not overwrite the captured default.
	write(t, cfg, "changed")
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(tool, DefaultName); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(cfg); string(got) != "c1" {
		t.Errorf("default not preserved: got %q", got)
	}
}

func TestSaveSwitchRoundTrip(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, auth := fakeTool(dir)
	write(t, cfg, "orig-cfg")
	write(t, auth, "orig-auth")

	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}

	// Change live config and save as "work".
	write(t, cfg, "work-cfg")
	write(t, auth, "work-auth")
	if err := s.Save(tool, "work", "Work", ""); err != nil {
		t.Fatal(err)
	}

	names := s.List("fake")
	if len(names) != 2 || names[0] != DefaultName {
		t.Errorf("List = %v, want [default work]", names)
	}

	// Switch back to default.
	if _, err := s.Apply(tool, DefaultName); err != nil {
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
	_ = s.EnsureDefault(tool)
	write(t, cfg, "current")
	backup, err := s.Apply(tool, DefaultName)
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

// TestSwitchingAwayRefreshesRotatingArtifact reproduces the "Claude Code always
// asks to log back in after switching back to default" bug: an OAuth token in the
// keychain rotates in the background, so a profile captured once and never
// refreshed goes stale by the time it's restored. Switching away from a profile
// must refresh its stored rotating artifact from the live value first.
func TestSwitchingAwayRefreshesRotatingArtifact(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, rot := rotatingTool(dir)
	write(t, cfg, "c1")
	tok1 := "token-v1"
	rot.value = &tok1

	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}

	// A second profile "work" already exists from some earlier point in time.
	write(t, cfg, "work-cfg")
	tokWork := "work-token"
	rot.value = &tokWork
	if err := s.Save(tool, "work", "Work", ""); err != nil {
		t.Fatal(err)
	}

	// Live state returns to what "default" (still active) actually looks like now:
	// its token has since rotated in the background, same as Claude Code silently
	// refreshing an OAuth session with no profile switch involved.
	write(t, cfg, "c1")
	rotated := "token-v2"
	rot.value = &rotated

	// Switching to "work" must refresh default's stored credentials to this live,
	// rotated value before leaving it — otherwise the stale token-v1 sticks around.
	if _, err := s.Apply(tool, "work"); err != nil {
		t.Fatal(err)
	}

	stored, err := os.ReadFile(filepath.Join(s.profDir("rotating", DefaultName), "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != "token-v2" {
		t.Errorf("default's stored credentials = %q, want refreshed value %q (not the stale first capture)", stored, "token-v2")
	}

	// Switching back to default must restore that refreshed token, not the stale one.
	if _, err := s.Apply(tool, DefaultName); err != nil {
		t.Fatal(err)
	}
	if rot.value == nil || *rot.value != "token-v2" {
		t.Errorf("live credentials after restoring default = %v, want token-v2", rot.value)
	}
}

// TestSwitchingAwayRefreshesOwnedKeysWithoutExplicitSave reproduces "I set /effort
// low + haiku on acc1, then set /effort mid + opus on acc2 — does switching between
// them save the config?": a profile-owned key (model/effort) changed live via /model
// or /effort, with no explicit `charon save`, must still be captured into the
// outgoing profile before it's left, the same way refreshKeychainArtifacts does for
// rotating credentials.
func TestSwitchingAwayRefreshesOwnedKeysWithoutExplicitSave(t *testing.T) {
	dir := t.TempDir()
	tool, cfg := mergedToolWithDisplay(dir)
	write(t, cfg, `{"model":"claude-haiku","effortLevel":"low"}`)

	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}
	write(t, cfg, `{"model":"claude-opus","effortLevel":"medium"}`)
	if err := s.Save(tool, "acc2", "Acc2", ""); err != nil {
		t.Fatal(err)
	}

	// Back on "default" (still active): the user runs /model and /effort live,
	// without ever calling `charon save`.
	write(t, cfg, `{"model":"claude-sonnet","effortLevel":"high"}`)

	// Switching to acc2 must capture that live edit into default's own snapshot first.
	if _, err := s.Apply(tool, "acc2"); err != nil {
		t.Fatal(err)
	}
	model, effort := s.ProfileModelEffort(tool, DefaultName)
	if model != "claude-sonnet" || effort != "high" {
		t.Errorf("default's captured model/effort = %q/%q, want claude-sonnet/high (the unsaved live edit)", model, effort)
	}

	// Switching back to default must restore that captured edit, not the original capture.
	if _, err := s.Apply(tool, DefaultName); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(cfg)
	var live map[string]any
	if err := json.Unmarshal(got, &live); err != nil {
		t.Fatal(err)
	}
	if live["model"] != "claude-sonnet" || live["effortLevel"] != "high" {
		t.Errorf("live after restoring default = %v, want claude-sonnet/high", live)
	}
}

// TestApplyPreservesLiveNonOwnedPreference reproduces "switching charon profiles
// resets Claude Code's /model or /effort choice": settings.json mixes a profile-owned
// field ("model") with a live CLI preference ("theme") that switching must not touch.
func TestApplyPreservesLiveNonOwnedPreference(t *testing.T) {
	dir := t.TempDir()
	tool, cfg := mergedTool(dir)
	write(t, cfg, `{"model":"default-model","theme":"dark"}`)

	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}
	write(t, cfg, `{"model":"work-model","theme":"dark"}`)
	if err := s.Save(tool, "work", "Work", ""); err != nil {
		t.Fatal(err)
	}

	// User changes their CLI preference live, unrelated to any profile switch.
	write(t, cfg, `{"model":"default-model","theme":"light"}`)

	if _, err := s.Apply(tool, "work"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(cfg)
	var after map[string]any
	if err := json.Unmarshal(got, &after); err != nil {
		t.Fatal(err)
	}
	if after["model"] != "work-model" {
		t.Errorf("model = %v, want work-model (owned key must switch per profile)", after["model"])
	}
	if after["theme"] != "light" {
		t.Errorf("theme = %v, want light (live preference must survive the switch)", after["theme"])
	}
}

// TestDriftIgnoresLiveNonOwnedPreference ensures a live-only preference change (e.g.
// /theme in a running Claude Code session) is not mistaken for external drift.
func TestDriftIgnoresLiveNonOwnedPreference(t *testing.T) {
	dir := t.TempDir()
	tool, cfg := mergedTool(dir)
	write(t, cfg, `{"model":"default-model","theme":"dark"}`)

	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}
	write(t, cfg, `{"model":"default-model","theme":"light"}`)
	if drift, err := s.Drift(tool); err != nil || drift {
		t.Fatalf("drift = %v, err = %v; want false, nil (preference-only change)", drift, err)
	}
	// A change to the owned field is real drift.
	write(t, cfg, `{"model":"changed-outside-charon","theme":"light"}`)
	if drift, err := s.Drift(tool); err != nil || !drift {
		t.Fatalf("drift = %v, err = %v; want true, nil (owned field changed)", drift, err)
	}
}

// TestProfileModelEffortReflectsEachProfilesOwnCapture reproduces "acc1 uses haiku +
// low effort, acc2 uses opus + medium effort, and switching between them should
// remember each": both model and effortLevel are owned keys, so each profile's own
// snapshot — not just whatever is live right now — must be readable independently.
func TestProfileModelEffortReflectsEachProfilesOwnCapture(t *testing.T) {
	dir := t.TempDir()
	tool, cfg := mergedToolWithDisplay(dir)
	write(t, cfg, `{"model":"claude-haiku","effortLevel":"low"}`)

	s := newStore(t)
	if err := s.Save(tool, "acc1", "", ""); err != nil {
		t.Fatal(err)
	}
	write(t, cfg, `{"model":"claude-opus","effortLevel":"medium"}`)
	if err := s.Save(tool, "acc2", "", ""); err != nil {
		t.Fatal(err)
	}

	model, effort := s.ProfileModelEffort(tool, "acc1")
	if model != "claude-haiku" || effort != "low" {
		t.Errorf("acc1: model=%q effort=%q, want claude-haiku/low", model, effort)
	}
	model, effort = s.ProfileModelEffort(tool, "acc2")
	if model != "claude-opus" || effort != "medium" {
		t.Errorf("acc2: model=%q effort=%q, want claude-opus/medium", model, effort)
	}

	// Switching to acc1 must restore its own model/effort, independent of acc2's.
	if _, err := s.Apply(tool, "acc1"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(cfg)
	var after map[string]any
	if err := json.Unmarshal(got, &after); err != nil {
		t.Fatal(err)
	}
	if after["model"] != "claude-haiku" || after["effortLevel"] != "low" {
		t.Errorf("live after switching to acc1 = %v, want haiku/low", after)
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
	if err := s.Remove("fake", DefaultName); err == nil {
		t.Error("removing default should fail")
	}
}

func TestSanitizeProfileName(t *testing.T) {
	cases := map[string]string{
		"alice@work.com":    "alice@work.com",
		"a b/c":             "a-b-c",
		"  spaced  ":        "spaced",
		"user+tag@x.io":     "user-tag@x.io",
		"acc_123.default-x": "acc_123.default-x",
		"///":               "",
		"":                  "",
		"多 bytes":           "bytes",
	}
	for in, want := range cases {
		if got := sanitizeProfileName(in); got != want {
			t.Errorf("sanitizeProfileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSaveCurrentAccount(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "live")
	tool.Describe = func() (tools.Info, error) {
		return tools.Info{Account: "alice@work.com"}, nil
	}

	s := newStore(t)
	name, err := s.SaveCurrentAccount(tool)
	if err != nil {
		t.Fatal(err)
	}
	if name != "alice@work.com" {
		t.Errorf("name = %q, want alice@work.com", name)
	}
	if !s.Exists("fake", "alice@work.com") {
		t.Error("account profile not saved")
	}
	if s.Active("fake") != "alice@work.com" {
		t.Errorf("active = %q, want alice@work.com", s.Active("fake"))
	}
	m, err := s.LoadManifest("fake", "alice@work.com")
	if err != nil {
		t.Fatal(err)
	}
	if m.Account != "alice@work.com" {
		t.Errorf("manifest account = %q, want alice@work.com", m.Account)
	}
}

func TestSaveCurrentAccountNoAccount(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "live")
	tool.Describe = func() (tools.Info, error) { return tools.Info{}, nil }

	s := newStore(t)
	if _, err := s.SaveCurrentAccount(tool); err == nil {
		t.Error("expected error when no account is detected")
	}
}

func TestUndoRevertsToPreSwitchState(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, auth := fakeTool(dir)
	write(t, cfg, "c1")
	write(t, auth, "a1")

	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil { // default = c1/a1, active=default
		t.Fatal(err)
	}
	// A second profile with different contents.
	write(t, cfg, "c2")
	write(t, auth, "a2")
	if err := s.Save(tool, "two", "", ""); err != nil {
		t.Fatal(err)
	}
	write(t, cfg, "c1")
	write(t, auth, "a1")

	// Switch to "two": backs up the current (default) state, then makes two live.
	if _, err := s.Apply(tool, "two"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(cfg); string(got) != "c2" {
		t.Fatalf("after switch cfg = %q, want c2", got)
	}

	// Undo restores the pre-switch (default) state and the active pointer with it.
	restored, err := s.Undo(tool)
	if err != nil {
		t.Fatal(err)
	}
	if restored == "" {
		t.Error("Undo returned empty restore path")
	}
	if got, _ := os.ReadFile(cfg); string(got) != "c1" {
		t.Errorf("after undo cfg = %q, want c1", got)
	}
	if s.Active("fake") != DefaultName {
		t.Errorf("after undo active = %q, want default", s.Active("fake"))
	}
}

func TestUndoNoBackups(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	if _, err := s.Undo(tool); err == nil {
		t.Error("expected error undoing with no backups")
	}
}

func TestPruneBackups(t *testing.T) {
	s := newStore(t)
	base := filepath.Join(s.Root, "backups", "fake")
	for _, stamp := range []string{"20200101-000001", "20200101-000002", "20200101-000003", "20200101-000004"} {
		if err := os.MkdirAll(filepath.Join(base, stamp), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := s.PruneBackups("fake", 2)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}
	if got := s.countBackups("fake"); got != 2 {
		t.Errorf("remaining = %d, want 2", got)
	}
	// The two newest must be the survivors.
	for _, stamp := range []string{"20200101-000003", "20200101-000004"} {
		if _, err := os.Stat(filepath.Join(base, stamp)); err != nil {
			t.Errorf("expected %s to survive prune", stamp)
		}
	}
}

func TestApplyCapsBackups(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c1")
	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(tool, "two", "", ""); err != nil {
		t.Fatal(err)
	}
	// Manufacture more old backups than the cap, then a switch must prune them.
	base := filepath.Join(s.Root, "backups", "fake")
	for i := 0; i < backupKeep+3; i++ {
		if err := os.MkdirAll(filepath.Join(base, "20200101-0000"+string(rune('0'+i%10))+string(rune('a'+i))), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Apply(tool, "two"); err != nil {
		t.Fatal(err)
	}
	if got := s.countBackups("fake"); got > backupKeep {
		t.Errorf("backups = %d, want <= %d", got, backupKeep)
	}
}

func TestDriftDetectsExternalChange(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, auth := fakeTool(dir)
	write(t, cfg, "c1")
	write(t, auth, "a1")

	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}
	// Freshly captured: live matches the active snapshot.
	if drift, err := s.Drift(tool); err != nil || drift {
		t.Fatalf("drift = %v, err = %v; want false, nil", drift, err)
	}
	// An external edit to the live config must register as drift.
	write(t, cfg, "changed-outside-charon")
	if drift, err := s.Drift(tool); err != nil || !drift {
		t.Fatalf("drift = %v, err = %v; want true, nil", drift, err)
	}
	// Re-applying the profile clears the drift.
	if _, err := s.Apply(tool, DefaultName); err != nil {
		t.Fatal(err)
	}
	if drift, _ := s.Drift(tool); drift {
		t.Error("drift should be cleared after re-apply")
	}
}

func TestDriftOnRemovedArtifact(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, auth := fakeTool(dir)
	write(t, cfg, "c1")
	write(t, auth, "a1")
	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(auth); err != nil { // artifact disappeared since the snapshot
		t.Fatal(err)
	}
	if drift, _ := s.Drift(tool); !drift {
		t.Error("removing a captured artifact should be drift")
	}
}

func TestRename(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	if err := s.Save(tool, "old", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveName("fake", "old"); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("fake", "old", "new"); err != nil {
		t.Fatal(err)
	}
	if s.Exists("fake", "old") {
		t.Error("old profile should be gone")
	}
	if !s.Exists("fake", "new") {
		t.Error("new profile should exist")
	}
	if s.Active("fake") != "new" {
		t.Errorf("active = %q, want new (pointer should follow rename)", s.Active("fake"))
	}
}

func TestRenameGuards(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}
	_ = s.Save(tool, "a", "", "")
	_ = s.Save(tool, "b", "", "")
	if err := s.Rename("fake", DefaultName, "x"); err == nil {
		t.Error("renaming default should fail")
	}
	if err := s.Rename("fake", "a", "b"); err == nil {
		t.Error("renaming onto an existing name should fail")
	}
	if err := s.Rename("fake", "a", DefaultName); err == nil {
		t.Error("renaming to the reserved default name should fail")
	}
	if err := s.Rename("fake", "missing", "x"); err == nil {
		t.Error("renaming a missing profile should fail")
	}
}

func TestDuplicate(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "proxy-config")
	s := newStore(t)
	// A proxy profile with a spec.
	if err := s.SaveWithSpec(tool, "proxy", Spec{Endpoint: "https://p/v1", Key: "sk-x", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	_ = s.SetActiveName("fake", "proxy")
	activeBefore := s.Active("fake")

	if err := s.Duplicate("fake", "proxy", "proxy-2"); err != nil {
		t.Fatal(err)
	}
	if !s.Exists("fake", "proxy-2") {
		t.Fatal("duplicate not created")
	}
	// The copy carries the spec (so it stays editable) and a fresh label.
	if sp, ok := s.GetSpec("fake", "proxy-2"); !ok || sp.Endpoint != "https://p/v1" {
		t.Errorf("duplicate spec = %+v, ok=%v; want the source spec", sp, ok)
	}
	if m, _ := s.LoadManifest("fake", "proxy-2"); m.Label != "proxy-2" {
		t.Errorf("label = %q, want proxy-2", m.Label)
	}
	// Duplication must not change which profile is active or the live config.
	if s.Active("fake") != activeBefore {
		t.Errorf("active changed to %q, want %q", s.Active("fake"), activeBefore)
	}
}

func TestDuplicateGuards(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	_ = s.EnsureDefault(tool)
	_ = s.Save(tool, "a", "", "")

	if err := s.Duplicate("fake", "a", "a"); err == nil {
		t.Error("duplicating onto an existing name should fail")
	}
	if err := s.Duplicate("fake", "a", DefaultName); err == nil {
		t.Error("duplicating onto the reserved default name should fail")
	}
	if err := s.Duplicate("fake", "missing", "x"); err == nil {
		t.Error("duplicating a missing source should fail")
	}
}
