package profile

import (
	"fmt"
	"os"
	"path/filepath"
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
			if err := artifact.AtomicWrite(filepath.Join(dir, a.ID()), data, 0o600); err != nil {
				return err
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
	if err := s.setActive(t.Name, name); err != nil {
		return "", err
	}
	return name, nil
}

// AddProfile applies spec via ApplyAuth, snapshots it as the named profile, and marks it
// active. Shared by the CLI `add` command and the interactive add/edit flow so they can't drift.
func (s *Store) AddProfile(t *tools.Tool, name string, spec Spec) error {
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
	if err := t.ApplyAuth(tools.AuthSpec{Endpoint: spec.Endpoint, Key: spec.Key, Model: spec.Model}); err != nil {
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
// old profile once the new one is safely in place. Shared by the CLI `edit` command
// and the TUI edit form so the rename cleanup can't drift between them.
func (s *Store) EditProfile(t *tools.Tool, oldName, newName string, spec Spec) error {
	if newName == "" {
		newName = oldName
	}
	if err := s.AddProfile(t, newName, spec); err != nil {
		return err
	}
	if oldName != newName {
		return s.Remove(t.Name, oldName)
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
// revert always works. It writes the reserved name directly — Save rejects it.
func (s *Store) EnsureDefault(t *tools.Tool) error {
	if s.Exists(t.Name, DefaultName) {
		return nil
	}
	if t.Detected == nil || !t.Detected() {
		return nil
	}
	if err := snapshot(t, s.profDir(t.Name, DefaultName), "Default (auto-captured)", "", "", "", nil); err != nil {
		return err
	}
	if s.Active(t.Name) == "" {
		return s.setActive(t.Name, DefaultName)
	}
	return nil
}
