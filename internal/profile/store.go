// Package profile stores named snapshots of each tool's auth surface and
// applies them, with automatic backups and an always-available "original".
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"charon/internal/tools"
)

// OriginalName is the reserved profile capturing config as first seen by charon.
const OriginalName = "original"

// Spec is the endpoint/key/model a profile was created from, recorded so the
// edit form can prefill its fields without re-parsing tool configs.
type Spec struct {
	Endpoint string `json:"endpoint,omitempty"`
	Key      string `json:"key,omitempty"`
	Model    string `json:"model,omitempty"`
}

// Manifest records metadata about a stored profile and which artifacts it
// contained (an artifact absent from the snapshot is restored by removal).
type Manifest struct {
	Label     string          `json:"label"`
	Note      string          `json:"note,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	Present   map[string]bool `json:"present"`
	Spec      *Spec           `json:"spec,omitempty"`
}

// Store is rooted at ~/.config/aies.
type Store struct {
	Root string
}

type config struct {
	Active map[string]string `json:"active"` // tool name -> profile name
}

// Open returns the store rooted at $XDG_CONFIG_HOME/aies (defaulting to
// ~/.config/aies), keeping it alongside the tools' own config directories.
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

// SetActiveName marks a profile active without applying files (used right after
// Save, when the live config already matches the profile).
func (s *Store) SetActiveName(tool, name string) error { return s.setActive(tool, name) }

// List returns stored profile names for a tool, "original" first.
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
		if names[i] == OriginalName {
			return true
		}
		if names[j] == OriginalName {
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
func snapshot(t *tools.Tool, dir string, label, note string, spec *Spec) error {
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
	m := Manifest{Label: label, Note: note, CreatedAt: time.Now(), Present: present, Spec: spec}
	data, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600)
}

// Save snapshots the tool's current live config as a named profile.
func (s *Store) Save(t *tools.Tool, name, label, note string) error {
	if label == "" {
		label = name
	}
	return snapshot(t, s.profDir(t.Name, name), label, note, nil)
}

// SaveWithSpec is like Save but also records the endpoint/key/model the profile
// was built from, so it can later be edited.
func (s *Store) SaveWithSpec(t *tools.Tool, name string, spec Spec) error {
	return snapshot(t, s.profDir(t.Name, name), name, "", &spec)
}

// GetSpec returns the recorded spec for a profile, if any.
func (s *Store) GetSpec(tool, name string) (Spec, bool) {
	m, err := s.LoadManifest(tool, name)
	if err != nil || m.Spec == nil {
		return Spec{}, false
	}
	return *m.Spec, true
}

// EnsureOriginal captures the pristine "original" profile the first time a
// detected tool is seen, so reverting is always possible.
func (s *Store) EnsureOriginal(t *tools.Tool) error {
	if s.Exists(t.Name, OriginalName) {
		return nil
	}
	if t.Detected == nil || !t.Detected() {
		return nil
	}
	if err := s.Save(t, OriginalName, "Original (auto-captured)", ""); err != nil {
		return err
	}
	if s.Active(t.Name) == "" {
		return s.setActive(t.Name, OriginalName)
	}
	return nil
}

// Apply restores a stored profile over the tool's live config, backing up the
// current state first. It marks the profile active on success.
func (s *Store) Apply(t *tools.Tool, name string) (backupDir string, err error) {
	if !s.Exists(t.Name, name) {
		return "", fmt.Errorf("profile %q not found for %s", name, t.Name)
	}

	// Back up current live state so the switch is reversible.
	stamp := time.Now().Format("20060102-150405")
	backupDir = filepath.Join(s.Root, "backups", t.Name, stamp)
	if err := snapshot(t, backupDir, "auto-backup before switch to "+name, "", nil); err != nil {
		return "", fmt.Errorf("backup failed, aborting: %w", err)
	}

	m, err := s.LoadManifest(t.Name, name)
	if err != nil {
		return "", err
	}
	pdir := s.profDir(t.Name, name)
	for _, a := range t.Artifacts {
		if m.Present[a.ID()] {
			data, rerr := os.ReadFile(filepath.Join(pdir, a.ID()))
			if rerr != nil {
				return backupDir, rerr
			}
			if werr := a.Write(data); werr != nil {
				return backupDir, werr
			}
		} else {
			// The profile had no such artifact; match it by removing the live one.
			if rerr := a.Remove(); rerr != nil {
				return backupDir, rerr
			}
		}
	}
	return backupDir, s.setActive(t.Name, name)
}

// Remove deletes a stored profile (the original cannot be removed).
func (s *Store) Remove(tool, name string) error {
	if name == OriginalName {
		return fmt.Errorf("the original profile cannot be removed")
	}
	return os.RemoveAll(s.profDir(tool, name))
}
