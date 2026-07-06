package tui

import (
	"strings"
	"testing"
)

func TestStatusRender(t *testing.T) {
	tests := []struct {
		name       string
		level      statusLevel
		msg        string
		wantEmpty  bool
		wantSubstr string // substring that must appear in the rendered line
	}{
		{name: "empty message renders nothing", level: statusOK, msg: "", wantEmpty: true},
		{name: "info has no glyph", level: statusInfo, msg: "cancelled", wantSubstr: "cancelled"},
		{name: "ok gets a check", level: statusOK, msg: "Switched", wantSubstr: "✓"},
		{name: "err gets a cross", level: statusErr, msg: "boom", wantSubstr: "✗"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusRender(tt.level, tt.msg)
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("statusRender(%v, %q) = %q, want empty", tt.level, tt.msg, got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSubstr) {
				t.Fatalf("statusRender(%v, %q) = %q, want substring %q", tt.level, tt.msg, got, tt.wantSubstr)
			}
			if !strings.Contains(got, tt.msg) {
				t.Fatalf("statusRender(%v, %q) = %q, want it to contain the message", tt.level, tt.msg, got)
			}
		})
	}
}

func TestOnQuitTwoStep(t *testing.T) {
	m := model{}

	// First ctrl+d arms the quit but does not exit.
	got, cmd := m.onQuit()
	armed := got.(model)
	if !armed.quitting {
		t.Fatal("first ctrl+d should arm quitting")
	}
	if cmd != nil {
		t.Fatal("first ctrl+d should not issue a quit command")
	}
	if armed.status == "" {
		t.Fatal("first ctrl+d should show a confirmation hint")
	}

	// A second ctrl+d (still armed) issues the quit command.
	if _, cmd2 := armed.onQuit(); cmd2 == nil {
		t.Fatal("second ctrl+d should issue a quit command")
	}
}

func TestWizardStep(t *testing.T) {
	tests := []struct {
		view      view
		wantN     int
		wantTotal int
		wantLabel string
	}{
		{viewAddEndpoint, 1, 4, "API base URL"},
		{viewAddKey, 2, 4, "API key"},
		{viewFetching, 3, 4, "choose a model"},
		{viewPickModel, 3, 4, "choose a model"},
		{viewAddName, 4, 4, "name the profile"},
		// Non-wizard views report no progress.
		{viewTools, 0, 0, ""},
		{viewProfiles, 0, 0, ""},
		{viewEditForm, 0, 0, ""},
		{viewEditField, 0, 0, ""},
		{viewSaveName, 0, 0, ""},
		{viewConfirmDelete, 0, 0, ""},
	}
	for _, tt := range tests {
		n, total, label := wizardStep(tt.view)
		if n != tt.wantN || total != tt.wantTotal || label != tt.wantLabel {
			t.Errorf("wizardStep(%v) = (%d, %d, %q), want (%d, %d, %q)",
				tt.view, n, total, label, tt.wantN, tt.wantTotal, tt.wantLabel)
		}
	}
}

func TestFilterModels(t *testing.T) {
	all := []string{"gpt-4o", "gpt-4o-mini", "claude-opus-4-8", "claude-sonnet-5", "o3-mini"}

	// An empty (or whitespace-only) query returns the full list unchanged.
	if got := filterModels(all, ""); len(got) != len(all) {
		t.Fatalf("empty query returned %d items, want %d", len(got), len(all))
	}
	if got := filterModels(all, "   "); len(got) != len(all) {
		t.Fatalf("whitespace query returned %d items, want %d", len(got), len(all))
	}

	// A query narrows to fuzzy matches only.
	got := filterModels(all, "claude")
	if len(got) != 2 {
		t.Fatalf("filterModels(claude) = %v, want 2 matches", got)
	}
	for _, id := range got {
		if !strings.Contains(id, "claude") {
			t.Fatalf("filterModels(claude) returned non-match %q", id)
		}
	}

	// Fuzzy (non-contiguous) matching works and ranks the closer id first.
	if got := filterModels(all, "gpt4o"); len(got) == 0 || got[0] != "gpt-4o" {
		t.Fatalf("filterModels(gpt4o) = %v, want best match gpt-4o", got)
	}

	// A query that matches nothing yields an empty result.
	if got := filterModels(all, "zzzz"); len(got) != 0 {
		t.Fatalf("filterModels(zzzz) = %v, want no matches", got)
	}
}

// TestWizardStepsAreSequential guards that the add-flow steps are numbered
// 1..total with a consistent total, so the progress line never lies.
func TestWizardStepsAreSequential(t *testing.T) {
	flow := []view{viewAddEndpoint, viewAddKey, viewPickModel, viewAddName}
	for i, v := range flow {
		n, total, _ := wizardStep(v)
		if total != len(flow) {
			t.Errorf("view %v: total = %d, want %d", v, total, len(flow))
		}
		if n != i+1 {
			t.Errorf("view %v: step = %d, want %d", v, n, i+1)
		}
	}
}
