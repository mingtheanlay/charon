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

// backupKeep is how many timestamped backups to retain per tool after a switch/undo.
const backupKeep = 10

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
	if m, merr := loadManifestFrom(dir); merr == nil {
		active = m.Active
	}
	return dir, active, nil
}

// loadManifestFrom reads the manifest.json inside an arbitrary snapshot dir.
func loadManifestFrom(dir string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
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
