package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Remove deletes a stored profile (the default cannot be removed).
func (s *Store) Remove(tool, name string) error {
	if name == DefaultName {
		return fmt.Errorf("the default profile cannot be removed")
	}
	if err := validateName(name); err != nil {
		return err
	}
	if !s.Exists(tool, name) {
		return fmt.Errorf("profile %q not found for %s", name, tool)
	}
	if err := os.RemoveAll(s.profDir(tool, name)); err != nil {
		return err
	}
	if s.Active(tool) == name {
		return s.setActive(tool, DefaultName)
	}
	return nil
}

// Rename moves a stored profile, updating the active pointer and a name-mirroring
// label to match. The default profile and name collisions are rejected.
func (s *Store) Rename(tool, old, dst string) error {
	if old == DefaultName {
		return fmt.Errorf("the default profile cannot be renamed")
	}
	if err := validateName(old); err != nil {
		return err
	}
	dst = sanitizeProfileName(dst)
	if err := validateNewName(dst); err != nil {
		return err
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
	s.refreshMirroredLabel(tool, old, dst)
	if s.Active(tool) == old {
		return s.setActive(tool, dst)
	}
	return nil
}

// Duplicate copies a stored profile to a new name (snapshot files + spec), leaving
// the live config and active pointer untouched. The default name is reserved and
// an existing target is rejected, so the copy is always a distinct, editable profile.
func (s *Store) Duplicate(tool, src, dst string) error {
	if err := validateName(src); err != nil {
		return err
	}
	dst = sanitizeProfileName(dst)
	if err := validateNewName(dst); err != nil {
		return err
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
	s.refreshMirroredLabel(tool, src, dst)
	return nil
}

// refreshMirroredLabel updates a moved/copied profile's label when it just
// mirrored the source name, so the display name follows the new one.
func (s *Store) refreshMirroredLabel(tool, src, dst string) {
	if m, err := s.LoadManifest(tool, dst); err == nil && (m.Label == src || m.Label == "") {
		m.Label = dst
		_ = writeManifest(s.profDir(tool, dst), m)
	}
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
