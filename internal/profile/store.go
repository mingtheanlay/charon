// Package profile stores and applies named snapshots of each tool's auth surface,
// with automatic backups and an always-available "default".
package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charon/internal/tools"
)

// DefaultName is the reserved profile capturing config as first seen by charon.
const DefaultName = "default"

// Spec is the endpoint/key/model a profile was created from, so the edit form can prefill.
type Spec struct {
	Endpoint string `json:"endpoint,omitempty"`
	Key      string `json:"key,omitempty"`
	Model    string `json:"model,omitempty"`
}

// Manifest records a stored profile's metadata and which artifacts it contained
// (an absent artifact is restored by removal).
type Manifest struct {
	Label     string          `json:"label"`
	Note      string          `json:"note,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	Present   map[string]bool `json:"present"`
	Spec      *Spec           `json:"spec,omitempty"`
	Account   string          `json:"account,omitempty"` // logged-in account this snapshot captured, if any
	Active    string          `json:"active,omitempty"`  // profile active when a backup was taken, for undo
}

// Store is rooted at ~/.config/charon.
type Store struct {
	Root string
}

type config struct {
	Active map[string]string `json:"active"` // tool name -> profile name
}

// Open returns the store rooted at $XDG_CONFIG_HOME/charon (default ~/.config/charon).
func Open() (*Store, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		base = filepath.Join(h, ".config")
	}
	root := filepath.Join(base, "charon")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Store{Root: root}, nil
}

func (s *Store) toolDir(tool string) string    { return filepath.Join(s.Root, "profiles", tool) }
func (s *Store) profDir(tool, n string) string { return filepath.Join(s.toolDir(tool), n) }
func (s *Store) configPath() string            { return filepath.Join(s.Root, "config.json") }

func (s *Store) readConfig() config {
	var c config
	if data, err := os.ReadFile(s.configPath()); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	if c.Active == nil {
		c.Active = map[string]string{}
	}
	return c
}

func (s *Store) writeConfig(c config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.configPath(), data, 0o600)
}

// Active returns the profile name currently marked active for a tool, or "".
func (s *Store) Active(tool string) string { return s.readConfig().Active[tool] }

func (s *Store) setActive(tool, name string) error {
	c := s.readConfig()
	c.Active[tool] = name
	return s.writeConfig(c)
}

// SetActiveName marks a profile active without applying files (used right after Save).
func (s *Store) SetActiveName(tool, name string) error { return s.setActive(tool, name) }

// List returns stored profile names for a tool, "default" first.
func (s *Store) List(tool string) []string {
	entries, err := os.ReadDir(s.toolDir(tool))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == DefaultName {
			return true
		}
		if names[j] == DefaultName {
			return false
		}
		return names[i] < names[j]
	})
	return names
}

// Exists reports whether a named profile is stored for a tool.
func (s *Store) Exists(tool, name string) bool {
	_, err := os.Stat(filepath.Join(s.profDir(tool, name), "manifest.json"))
	return err == nil
}

// LoadManifest reads a stored profile's metadata.
func (s *Store) LoadManifest(tool, name string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(s.profDir(tool, name), "manifest.json"))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

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
			if err := os.WriteFile(filepath.Join(dir, a.ID()), data, 0o600); err != nil {
				return err
			}
		}
	}
	m := Manifest{Label: label, Note: note, CreatedAt: time.Now(), Present: present, Spec: spec, Account: account, Active: active}
	data, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600)
}

// Save snapshots the tool's current live config as a named profile.
func (s *Store) Save(t *tools.Tool, name, label, note string) error {
	if label == "" {
		label = name
	}
	return snapshot(t, s.profDir(t.Name, name), label, note, "", "", nil)
}

// SaveWithSpec is Save plus the endpoint/key/model the profile was built from, for editing.
func (s *Store) SaveWithSpec(t *tools.Tool, name string, spec Spec) error {
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
	if name == "" {
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

// sanitizeProfileName maps an account identity to a filesystem-safe profile name,
// keeping [A-Za-z0-9._@-] and replacing every other run with a single "-".
func sanitizeProfileName(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '@', r == '-':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// AddProfile applies spec via ApplyAuth, snapshots it as the named profile, and marks it
// active. Shared by the CLI `add` command and the interactive add/edit flow so they can't drift.
func (s *Store) AddProfile(t *tools.Tool, name string, spec Spec) error {
	if t.ApplyAuth == nil {
		return fmt.Errorf("%s does not support add", t.Title)
	}
	// Back up the current live config first so the write is reversible via undo.
	if t.Detected != nil && t.Detected() {
		s.refreshKeychainArtifacts(t)
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

// GetSpec returns the recorded spec for a profile, if any.
func (s *Store) GetSpec(tool, name string) (Spec, bool) {
	m, err := s.LoadManifest(tool, name)
	if err != nil || m.Spec == nil {
		return Spec{}, false
	}
	return *m.Spec, true
}

// EnsureDefault captures the "default" profile the first time a tool is seen, so revert always works.
func (s *Store) EnsureDefault(t *tools.Tool) error {
	if s.Exists(t.Name, DefaultName) {
		return nil
	}
	if t.Detected == nil || !t.Detected() {
		return nil
	}
	if err := s.Save(t, DefaultName, "Default (auto-captured)", ""); err != nil {
		return err
	}
	if s.Active(t.Name) == "" {
		return s.setActive(t.Name, DefaultName)
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
		r, ok := a.(tools.Rotator)
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
			_ = os.WriteFile(filepath.Join(dir, a.ID()), data, 0o600)
		}
	}
	if changed {
		if out, err := json.MarshalIndent(m, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(dir, "manifest.json"), out, 0o600)
		}
	}
}

// backupKeep is how many timestamped backups to retain per tool after a switch/undo.
const backupKeep = 10

// Apply restores a stored profile over the live config (backing up current first) and marks it active.
func (s *Store) Apply(t *tools.Tool, name string) (backupDir string, err error) {
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

// backup snapshots the tool's current live config into a fresh timestamped backup
// dir, recording the active profile so Undo can restore it. Returns the dir.
func (s *Store) backup(t *tools.Tool, label string) (string, error) {
	dir := s.uniqueBackupDir(t.Name)
	if err := snapshot(t, dir, label, "", "", s.Active(t.Name), nil); err != nil {
		return "", err
	}
	return dir, nil
}

// uniqueBackupDir returns a fresh backup path for a tool. It keeps the readable
// timestamp name, appending "-2", "-3"… only when two backups land in the same
// second — the suffix still sorts chronologically after the bare stamp.
func (s *Store) uniqueBackupDir(tool string) string {
	base := filepath.Join(s.Root, "backups", tool)
	stamp := time.Now().Format("20060102-150405")
	dir := filepath.Join(base, stamp)
	for n := 2; ; n++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return dir
		}
		dir = filepath.Join(base, fmt.Sprintf("%s-%d", stamp, n))
	}
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
			if merger, ok := a.(tools.Merger); ok {
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

// latestBackup returns the newest backup dir for a tool and the profile that was
// active when it was taken. It errors when the tool has no backups.
func (s *Store) latestBackup(tool string) (dir, active string, err error) {
	base := filepath.Join(s.Root, "backups", tool)
	entries, rerr := os.ReadDir(base)
	if rerr != nil {
		return "", "", fmt.Errorf("nothing to undo for %s", tool)
	}
	var stamps []string
	for _, e := range entries {
		if e.IsDir() {
			stamps = append(stamps, e.Name())
		}
	}
	if len(stamps) == 0 {
		return "", "", fmt.Errorf("nothing to undo for %s", tool)
	}
	sort.Strings(stamps) // timestamp names sort chronologically
	newest := stamps[len(stamps)-1]
	dir = filepath.Join(base, newest)
	if data, rerr := os.ReadFile(filepath.Join(dir, "manifest.json")); rerr == nil {
		var m Manifest
		if json.Unmarshal(data, &m) == nil {
			active = m.Active
		}
	}
	return dir, active, nil
}

// pruneBackups removes all but the newest keep backups for a tool.
func (s *Store) pruneBackups(tool string, keep int) error {
	base := filepath.Join(s.Root, "backups", tool)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil // no backups yet
	}
	var stamps []string
	for _, e := range entries {
		if e.IsDir() {
			stamps = append(stamps, e.Name())
		}
	}
	if len(stamps) <= keep {
		return nil
	}
	sort.Strings(stamps) // oldest first
	for _, old := range stamps[:len(stamps)-keep] {
		if err := os.RemoveAll(filepath.Join(base, old)); err != nil {
			return err
		}
	}
	return nil
}

// PruneBackups removes all but the newest keep backups for a tool (keep<0 uses the default).
func (s *Store) PruneBackups(tool string, keep int) (int, error) {
	if keep < 0 {
		keep = backupKeep
	}
	before := s.countBackups(tool)
	if err := s.pruneBackups(tool, keep); err != nil {
		return 0, err
	}
	return before - s.countBackups(tool), nil
}

// countBackups returns how many timestamped backups a tool currently has.
func (s *Store) countBackups(tool string) int {
	entries, err := os.ReadDir(filepath.Join(s.Root, "backups", tool))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n
}

// Remove deletes a stored profile (the default cannot be removed).
func (s *Store) Remove(tool, name string) error {
	if name == DefaultName {
		return fmt.Errorf("the default profile cannot be removed")
	}
	return os.RemoveAll(s.profDir(tool, name))
}

// Rename moves a stored profile, updating the active pointer and a name-mirroring
// label to match. The default profile and name collisions are rejected.
func (s *Store) Rename(tool, old, dst string) error {
	if old == DefaultName {
		return fmt.Errorf("the default profile cannot be renamed")
	}
	dst = sanitizeProfileName(dst)
	if dst == "" {
		return fmt.Errorf("invalid new profile name")
	}
	if dst == DefaultName {
		return fmt.Errorf("%q is a reserved name", DefaultName)
	}
	if !s.Exists(tool, old) {
		return fmt.Errorf("profile %q not found for %s", old, tool)
	}
	if s.Exists(tool, dst) {
		return fmt.Errorf("profile %q already exists", dst)
	}
	if err := os.Rename(s.profDir(tool, old), s.profDir(tool, dst)); err != nil {
		return err
	}
	// Refresh a label that just mirrored the old name.
	if m, err := s.LoadManifest(tool, dst); err == nil && (m.Label == old || m.Label == "") {
		m.Label = dst
		if data, err := json.MarshalIndent(m, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(s.profDir(tool, dst), "manifest.json"), data, 0o600)
		}
	}
	if s.Active(tool) == old {
		return s.setActive(tool, dst)
	}
	return nil
}

// Duplicate copies a stored profile to a new name (snapshot files + spec), leaving
// the live config and active pointer untouched. The default name is reserved and
// an existing target is rejected, so the copy is always a distinct, editable profile.
func (s *Store) Duplicate(tool, src, dst string) error {
	dst = sanitizeProfileName(dst)
	if dst == "" {
		return fmt.Errorf("invalid profile name")
	}
	if dst == DefaultName {
		return fmt.Errorf("%q is a reserved name", DefaultName)
	}
	if !s.Exists(tool, src) {
		return fmt.Errorf("profile %q not found for %s", src, tool)
	}
	if s.Exists(tool, dst) {
		return fmt.Errorf("profile %q already exists", dst)
	}
	if err := copyDir(s.profDir(tool, src), s.profDir(tool, dst)); err != nil {
		return err
	}
	// Refresh a label that just mirrored the source name.
	if m, err := s.LoadManifest(tool, dst); err == nil && (m.Label == src || m.Label == "") {
		m.Label = dst
		if data, err := json.MarshalIndent(m, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(s.profDir(tool, dst), "manifest.json"), data, 0o600)
		}
	}
	return nil
}

// copyDir copies the files of a profile directory (profiles are flat) into dst.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o600); err != nil {
			return err
		}
	}
	return nil
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
		if merger, ok := a.(tools.Merger); ok {
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
