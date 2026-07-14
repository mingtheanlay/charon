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
	if oldName == DefaultName {
		if newName != DefaultName {
			return fmt.Errorf("the default profile cannot be renamed")
		}
		if _, ok := s.GetSpec(t.Name, DefaultName); !ok {
			return fmt.Errorf("the official default profile cannot be edited")
		}
		if t.ApplyAuth == nil {
			return fmt.Errorf("%s does not support edit", t.Title)
		}
		if t.Detected != nil && t.Detected() {
			if _, err := s.backup(t, "auto-backup before editing default"); err != nil {
				return fmt.Errorf("backup failed, aborting: %w", err)
			}
		}
		if err := t.ApplyAuth(tools.AuthSpec{Endpoint: spec.Endpoint, Key: spec.Key, Model: spec.Model, AllModels: allModels}); err != nil {
			return err
		}
		return snapshot(t, s.profDir(t.Name, DefaultName), "Default (auto-captured custom provider)", "", "", "", &spec)
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

// EnsureDefault captures a clean official default the first time a tool is seen.
// A live custom provider is imported as a normal editable profile and left active.
func (s *Store) EnsureDefault(t *tools.Tool) error {
	if s.Exists(t.Name, DefaultName) {
		if _, custom := s.GetSpec(t.Name, DefaultName); custom && t.UseOfficialAuth != nil {
			return s.splitCustomDefault(t)
		}
		// Never override an explicitly active custom profile merely because OAuth
		// credentials also exist. Switching to default performs official cleanup.
		if s.Active(t.Name) == DefaultName && t.OfficialOAuth != nil && t.OfficialOAuth() && t.UseOfficialAuth != nil && s.liveUsesCustomEndpoint(t) {
			return s.activateOfficialOAuth(t)
		}
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
		if endpoint != "" && !strings.Contains(endpoint, "(default)") && endpoint != defaultEndpoint && t.UseOfficialAuth != nil {
			return s.importCustomAndCreateDefault(t, Spec{Endpoint: info.Endpoint, Key: info.Secret, Model: info.Model})
		}
	}
	if err := snapshot(t, s.profDir(t.Name, DefaultName), "Default (official provider)", "", "", "", nil); err != nil {
		return err
	}
	if s.Active(t.Name) == "" {
		return s.setActive(t.Name, DefaultName)
	}
	return nil
}

func (s *Store) liveUsesCustomEndpoint(t *tools.Tool) bool {
	if t.Describe == nil {
		return false
	}
	info, err := t.Describe()
	if err != nil {
		return false
	}
	endpoint := strings.TrimRight(info.Endpoint, "/")
	defaultEndpoint := strings.TrimRight(t.DefaultEndpoint, "/")
	return endpoint != "" && !strings.Contains(endpoint, "(default)") && endpoint != defaultEndpoint
}

// activateOfficialOAuth removes stale custom routing after an official login,
// preserving the imported profile and newly-created OAuth credentials.
func (s *Store) activateOfficialOAuth(t *tools.Tool) error {
	if _, err := s.backup(t, "auto-backup before activating official OAuth"); err != nil {
		return fmt.Errorf("backup failed, aborting: %w", err)
	}
	if err := t.UseOfficialAuth(); err != nil {
		return fmt.Errorf("activating official OAuth: %w", err)
	}
	if err := snapshot(t, s.profDir(t.Name, DefaultName), "Default (official OAuth)", "", "", "", nil); err != nil {
		return err
	}
	return s.setActive(t.Name, DefaultName)
}

func (s *Store) nextImportedName(tool string) string {
	name := "imported"
	for i := 2; s.Exists(tool, name); i++ {
		name = fmt.Sprintf("imported-%d", i)
	}
	return name
}

// importCustomAndCreateDefault temporarily clears custom routing to snapshot a
// clean official default, then restores the imported custom profile as active.
func (s *Store) importCustomAndCreateDefault(t *tools.Tool, spec Spec) error {
	name := s.nextImportedName(t.Name)
	if err := snapshot(t, s.profDir(t.Name, name), name, "Auto-imported custom provider", "", "", &spec); err != nil {
		return err
	}
	if err := t.UseOfficialAuth(); err != nil {
		return fmt.Errorf("creating official default: %w", err)
	}
	if err := snapshot(t, s.profDir(t.Name, DefaultName), "Default (official provider)", "", "", "", nil); err != nil {
		_ = s.restoreFrom(t, s.profDir(t.Name, name))
		return err
	}
	if err := s.restoreFrom(t, s.profDir(t.Name, name)); err != nil {
		return fmt.Errorf("restoring imported custom provider: %w", err)
	}
	return s.setActive(t.Name, name)
}

// splitCustomDefault migrates custom defaults created by earlier builds into the
// same imported + clean official layout used for new installations.
func (s *Store) splitCustomDefault(t *tools.Tool) error {
	name := s.nextImportedName(t.Name)
	if err := os.Rename(s.profDir(t.Name, DefaultName), s.profDir(t.Name, name)); err != nil {
		return fmt.Errorf("preserving custom default as %q: %w", name, err)
	}
	m, err := s.LoadManifest(t.Name, name)
	if err != nil {
		return err
	}
	m.Label = name
	if err := writeManifest(s.profDir(t.Name, name), m); err != nil {
		return err
	}
	if err := t.UseOfficialAuth(); err != nil {
		_ = os.Rename(s.profDir(t.Name, name), s.profDir(t.Name, DefaultName))
		return fmt.Errorf("creating official default: %w", err)
	}
	if err := snapshot(t, s.profDir(t.Name, DefaultName), "Default (official provider)", "", "", "", nil); err != nil {
		return err
	}
	if err := s.restoreFrom(t, s.profDir(t.Name, name)); err != nil {
		return err
	}
	return s.setActive(t.Name, name)
}
