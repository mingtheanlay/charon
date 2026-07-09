package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"charon/internal/artifact"
	"charon/internal/tools"
)

// Apply restores a stored profile over the live config (backing up current first) and marks it active.
func (s *Store) Apply(t *tools.Tool, name string) (backupDir string, err error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	if !s.Exists(t.Name, name) {
		return "", fmt.Errorf("profile %q not found for %s", name, t.Name)
	}
	return s.switchTo(t, s.profDir(t.Name, name), "auto-backup before switch to "+name, name)
}

// Undo reverts a tool to its most recent backup (the pre-switch state), snapshotting
// the current state first so the undo is itself reversible. Returns the restored dir.
func (s *Store) Undo(t *tools.Tool) (restoredFrom string, err error) {
	target, prevActive, err := s.latestBackup(t.Name)
	if err != nil {
		return "", err
	}
	return s.switchTo(t, target, "auto-backup before undo", prevActive)
}

// switchTo backs up the tool's current live state, restores sourceDir over it, and
// marks resultingActive active (skipped when empty, as Undo has none to restore
// from a backup taken before charon tracked the active profile). Shared by Apply
// and Undo so the backup→restore→prune sequence can't drift between them.
func (s *Store) switchTo(t *tools.Tool, sourceDir, backupLabel, resultingActive string) (backupDir string, err error) {
	s.refreshKeychainArtifacts(t)
	s.refreshMergerArtifacts(t)
	backupDir, err = s.backup(t, backupLabel)
	if err != nil {
		return "", fmt.Errorf("backup failed, aborting: %w", err)
	}
	if err := s.restoreFrom(t, sourceDir); err != nil {
		return backupDir, err
	}
	if resultingActive != "" {
		if err := s.setActive(t.Name, resultingActive); err != nil {
			return backupDir, err
		}
	}
	_ = s.pruneBackups(t.Name, backupKeep)
	return backupDir, nil
}

// restoreFrom overwrites the tool's live artifacts with a snapshot dir's contents,
// removing any artifact the snapshot didn't contain.
func (s *Store) restoreFrom(t *tools.Tool, dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for _, a := range t.Artifacts {
		if m.Present[a.ID()] {
			data, rerr := os.ReadFile(filepath.Join(dir, a.ID()))
			if rerr != nil {
				return rerr
			}
			if merger, ok := a.(artifact.Merger); ok {
				if live, _, rerr := a.Read(); rerr == nil {
					if merged, merr := merger.Merge(data, live); merr == nil {
						data = merged
					}
				}
			}
			if werr := a.Write(data); werr != nil {
				return werr
			}
		} else {
			// The snapshot had no such artifact; match it by removing the live one.
			if rerr := a.Remove(); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}

// refreshKeychainArtifacts overwrites the currently active profile's stored copy of
// any keychain-backed artifact with its current live value, before that profile is
// left. OS keychain entries commonly hold OAuth tokens the underlying tool silently
// refreshes/rotates in the background (e.g. Claude Code's login) — freezing one at
// first capture makes the profile's login go stale, forcing a fresh login the next
// time it's restored. Ordinary config-file artifacts are untouched: Apply's revert-
// to-snapshot behavior for those (see Drift) is deliberate, not staleness to fix.
func (s *Store) refreshKeychainArtifacts(t *tools.Tool) {
	active := s.Active(t.Name)
	if active == "" || !s.Exists(t.Name, active) {
		return
	}
	m, err := s.LoadManifest(t.Name, active)
	if err != nil {
		return
	}
	dir := s.profDir(t.Name, active)
	changed := false
	for _, a := range t.Artifacts {
		r, ok := a.(artifact.Rotator)
		if !ok || !r.Rotates() {
			continue
		}
		data, exists, err := a.Read()
		if err != nil {
			continue
		}
		if exists != m.Present[a.ID()] {
			m.Present[a.ID()] = exists
			changed = true
		}
		if exists {
			_ = artifact.AtomicWrite(filepath.Join(dir, a.ID()), data, 0o600)
		}
	}
	if changed {
		_ = writeManifest(dir, m)
	}
}

// refreshMergerArtifacts overwrites the currently active profile's stored copy of any
// Merger config-file artifact's owned keys (e.g. model, effort) with their current
// live values, before that profile is left — so an in-session change like /model or
// /effort isn't lost the next time this profile is restored, without requiring an
// explicit `save` first. Mirrors refreshKeychainArtifacts for rotating credentials.
func (s *Store) refreshMergerArtifacts(t *tools.Tool) {
	active := s.Active(t.Name)
	if active == "" || !s.Exists(t.Name, active) {
		return
	}
	dir := s.profDir(t.Name, active)
	for _, a := range t.Artifacts {
		merger, ok := a.(artifact.Merger)
		if !ok {
			continue
		}
		live, exists, err := a.Read()
		if err != nil || !exists {
			continue
		}
		storedPath := filepath.Join(dir, a.ID())
		stored, err := os.ReadFile(storedPath)
		if err != nil {
			continue
		}
		// Merge(live, stored) keeps stored's shape but takes owned keys from live —
		// the mirror image of restore, which keeps live's shape and takes owned keys
		// from the snapshot.
		refreshed, err := merger.Merge(live, stored)
		if err != nil {
			continue
		}
		_ = artifact.AtomicWrite(storedPath, refreshed, 0o600)
	}
}

// Drift reports whether the active profile's snapshot differs from the live config
// — e.g. an external `claude login` or a hand edit changed things since the switch.
func (s *Store) Drift(t *tools.Tool) (bool, error) {
	active := s.Active(t.Name)
	if active == "" || !s.Exists(t.Name, active) {
		return false, nil
	}
	m, err := s.LoadManifest(t.Name, active)
	if err != nil {
		return false, err
	}
	pdir := s.profDir(t.Name, active)
	for _, a := range t.Artifacts {
		live, liveExists, err := a.Read()
		if err != nil {
			return false, err
		}
		stored := m.Present[a.ID()]
		if stored != liveExists {
			return true, nil // artifact appeared or disappeared
		}
		if !stored {
			continue
		}
		want, err := os.ReadFile(filepath.Join(pdir, a.ID()))
		if err != nil {
			return false, err
		}
		if merger, ok := a.(artifact.Merger); ok {
			// Compare canonically re-encoded forms so Merge's own formatting (key order,
			// indentation) can't itself register as drift.
			if expected, merr := merger.Merge(want, live); merr == nil {
				if canonicalLive, lerr := merger.Merge(live, live); lerr == nil {
					want, live = expected, canonicalLive
				}
			}
		}
		if !bytes.Equal(want, live) {
			return true, nil
		}
	}
	return false, nil
}
