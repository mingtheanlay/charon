package profile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charon/internal/artifact"
	"charon/internal/tools"
)

// snapshot captures the tool's current live artifacts into dir.
func snapshot(t *tools.Tool, dir string, label, note, account, active string, spec *Spec) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	present := map[string]bool{}
	for _, a := range t.Artifacts {
		data, exists, err := a.Read()
		if err != nil {
			return fmt.Errorf("reading %s: %w", a.ID(), err)
		}
		present[a.ID()] = exists
		if exists {
			path := filepath.Join(dir, a.ID())
			if err := artifact.AtomicWrite(path, data, 0o600); err != nil {
				return err
			}
			// Verification step: read it back and verify content matches exactly
			backedData, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("verifying backup of %s: failed to read written file: %w", a.ID(), err)
			}
			if !bytes.Equal(backedData, data) {
				return fmt.Errorf("verifying backup of %s: content mismatch", a.ID())
			}
		}
	}
	m := Manifest{Label: label, Note: note, CreatedAt: time.Now(), Present: present, Spec: spec, Account: account, Active: active}
	return writeManifest(dir, m)
}

// Save snapshots the tool's current live config as a named profile.
func (s *Store) Save(t *tools.Tool, name, label, note string) error {
	if err := validateNewName(name); err != nil {
		return err
	}
	if label == "" {
		label = name
	}
	return snapshot(t, s.profDir(t.Name, name), label, note, "", "", nil)
}

// SaveWithSpec is Save plus the endpoint/key/model the profile was built from, for editing.
func (s *Store) SaveWithSpec(t *tools.Tool, name string, spec Spec) error {
	if err := validateNewName(name); err != nil {
		return err
	}
	return snapshot(t, s.profDir(t.Name, name), name, "", "", "", &spec)
}

// SaveCurrentAccount snapshots the tool's live config under a profile named after
// the logged-in account (its email), marks it active, and returns that name. It
// errors when no OAuth account is detected, so the caller can ask for a name.
func (s *Store) SaveCurrentAccount(t *tools.Tool) (string, error) {
	if t.Describe == nil {
		return "", fmt.Errorf("%s cannot report an account", t.Title)
	}
	info, err := t.Describe()
	if err != nil {
		return "", err
	}
	if info.Account == "" {
		return "", fmt.Errorf("no logged-in account detected for %s; pass a profile name", t.Title)
	}
	name := sanitizeProfileName(info.Account)
	if validateNewName(name) != nil {
		return "", fmt.Errorf("account %q is not a usable profile name", info.Account)
	}
	if err := snapshot(t, s.profDir(t.Name, name), info.Account, "", info.Account, "", nil); err != nil {
		return "", err
	}
	return name, nil
}

// AddProfile applies spec via ApplyAuth, snapshots it as the named profile, and marks it
// active. Shared by the CLI `add` command and the interactive add/edit flow so they can't drift.
// allModels is an optional full model list (e.g. from the TUI wizard's picker fetch) embedded
// into the tool's own config so its native model picker can offer more than just spec.Model;
// it is never persisted into the profile's Spec.
func (s *Store) AddProfile(t *tools.Tool, name string, spec Spec, allModels ...string) error {
	if t.ApplyAuth == nil {
		return fmt.Errorf("%s does not support add", t.Title)
	}
	// Reject a bad name before ApplyAuth so the live config is never touched for it.
	if err := validateNewName(name); err != nil {
		return err
	}
	// Back up the current live config first so the write is reversible via undo.
	if t.Detected != nil && t.Detected() {
		s.refreshKeychainArtifacts(t)
		s.refreshMergerArtifacts(t)
		if _, err := s.backup(t, "auto-backup before adding "+name); err != nil {
			return fmt.Errorf("backup failed, aborting: %w", err)
		}
	}
	if err := t.ApplyAuth(tools.AuthSpec{Endpoint: spec.Endpoint, Key: spec.Key, Model: spec.Model, AllModels: allModels}); err != nil {
		return err
	}
	if err := s.SaveWithSpec(t, name, spec); err != nil {
		return fmt.Errorf("applied config but failed to record profile: %w", err)
	}
	if err := s.setActive(t.Name, name); err != nil {
		return err
	}
	_ = s.pruneBackups(t.Name, backupKeep)
	return nil
}

// EditProfile re-applies spec under newName and, when this is a rename, removes the
// old profile once the new one is safely in place. Editing the profile that is
// currently active also updates the live config, since you're changing what's in
// use; editing any other saved profile only updates its stored spec — the live
// config and active pointer are restored to what they were, so an edit never
// silently switches which profile is in effect. Shared by the CLI `edit` command
// and the TUI edit form so this can't drift between them.
func (s *Store) EditProfile(t *tools.Tool, oldName, newName string, spec Spec, allModels ...string) error {
	if newName == "" {
		newName = oldName
	}
	prevActive := s.Active(t.Name)
	wasActive := prevActive == oldName

	if err := s.AddProfile(t, newName, spec, allModels...); err != nil {
		return err
	}
	if oldName != newName {
		if err := s.Remove(t.Name, oldName); err != nil {
			return err
		}
	}
	if !wasActive && prevActive != "" && prevActive != newName && s.Exists(t.Name, prevActive) {
		if _, err := s.Apply(t, prevActive); err != nil {
			return err
		}
	}
	return nil
}

// GetSpec returns the recorded spec for a profile, if any.
func (s *Store) GetSpec(tool, name string) (Spec, bool) {
	m, err := s.LoadManifest(tool, name)
	if err != nil || m.Spec == nil {
		return Spec{}, false
	}
	return *m.Spec, true
}

// EnsureDefault captures the "default" profile the first time a tool is seen, so
// revert always works. Custom provider configs are not captured under the reserved,
// immutable name. It writes the reserved name directly — Save rejects it.
func (s *Store) EnsureDefault(t *tools.Tool) error {
	if s.Exists(t.Name, DefaultName) {
		return nil
	}
	if t.Detected == nil || !t.Detected() {
		return nil
	}
	if t.Describe != nil {
		info, err := t.Describe()
		if err != nil {
			return fmt.Errorf("describing %s before capturing default: %w", t.Title, err)
		}
		endpoint := strings.TrimRight(info.Endpoint, "/")
		defaultEndpoint := strings.TrimRight(t.DefaultEndpoint, "/")
		if endpoint != "" && !strings.Contains(endpoint, "(default)") && endpoint != defaultEndpoint {
			return nil
		}
	}
	if err := snapshot(t, s.profDir(t.Name, DefaultName), "Default (auto-captured)", "", "", "", nil); err != nil {
		return err
	}
	if s.Active(t.Name) == "" {
		return s.setActive(t.Name, DefaultName)
	}
	return nil
}
