package profile

import (
	"testing"
)

func TestRefreshNoActiveProfileIsNoop(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	// No EnsureDefault/Apply has run yet, so nothing is active.
	if err := s.Refresh(tool); err != nil {
		t.Fatalf("Refresh with no active profile should be a no-op, got %v", err)
	}
}

func TestRefreshCapturesLiveChangeIntoActiveProfile(t *testing.T) {
	dir := t.TempDir()
	tool, cfg := mergedToolWithDisplay(dir)
	write(t, cfg, `{"model":"claude-haiku","effortLevel":"low"}`)

	s := newStore(t)
	if err := s.EnsureDefault(tool); err != nil {
		t.Fatal(err)
	}

	// Live /model change with no explicit save and no profile switch.
	write(t, cfg, `{"model":"claude-opus","effortLevel":"high"}`)
	if err := s.Refresh(tool); err != nil {
		t.Fatal(err)
	}

	model, effort := s.ProfileModelEffort(tool, DefaultName)
	if model != "claude-opus" || effort != "high" {
		t.Errorf("after Refresh, default's captured model/effort = %q/%q, want claude-opus/high", model, effort)
	}
}

func TestApplyRejectsInvalidName(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	if _, err := s.Apply(tool, "../escape"); err == nil {
		t.Error("expected error applying an invalid profile name")
	}
}

func TestApplyMissingProfile(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	if _, err := s.Apply(tool, "nonexistent"); err == nil {
		t.Error("expected error applying a profile that was never saved")
	}
}

func TestDriftNoActiveProfile(t *testing.T) {
	dir := t.TempDir()
	tool, cfg, _ := fakeTool(dir)
	write(t, cfg, "c")
	s := newStore(t)
	drift, err := s.Drift(tool)
	if err != nil || drift {
		t.Errorf("Drift with no active profile = %v, %v; want false, nil", drift, err)
	}
}
