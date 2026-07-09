// Package artifact provides the snapshot/restore primitives for a tool's auth
// surface: files on disk and OS keychain entries, plus the merge/rotate/peek
// behaviors the profile store keys off.
package artifact

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"

	"charon/internal/secret"
)

// Artifact is a single piece of a tool's auth surface that can be snapshotted
// and restored — either a file on disk or an OS keychain entry.
type Artifact interface {
	// ID is a stable, filesystem-safe name used to store this artifact
	// inside a profile directory.
	ID() string
	// Read returns the artifact's bytes and whether it currently exists.
	Read() (data []byte, exists bool, err error)
	// Write creates or replaces the artifact with data.
	Write(data []byte) error
	// Remove deletes the artifact; a missing artifact is not an error.
	Remove() error
}

// Rotator is implemented by artifacts whose live contents can change independent of
// any profile switch — e.g. an OS keychain entry holding an OAuth token that the
// underlying tool silently refreshes/rotates in the background. The store uses this
// to keep such an artifact's stored snapshot fresh whenever its profile is about to
// be left, so restoring it later doesn't hand back an already-invalidated token.
type Rotator interface {
	Rotates() bool
}

// Merger is implemented by an artifact whose restore should merge the snapshot with
// the live file rather than fully overwrite it — e.g. a config file that mixes
// profile-owned fields (auth/model routing) with CLI-level user preferences (theme,
// reasoning effort, etc.) that should survive every profile switch instead of
// reverting to whatever a profile last captured.
type Merger interface {
	// Merge returns what should be written: snapshotData, but with every top-level
	// key not in the artifact's owned set taken from liveData instead.
	Merge(snapshotData, liveData []byte) ([]byte, error)
}

// Peeker is implemented by an artifact that can report a human-readable model/effort
// summary from a stored snapshot's raw bytes, without applying it — so the profile
// list can show each profile's own captured model and reasoning effort.
type Peeker interface {
	// Peek returns data's model/effort values, each "" if not tracked or unset.
	Peek(data []byte) (model, effort string)
}

// FileArtifact is a config or credential file owned by a tool.
type FileArtifact struct {
	id       string
	Path     string
	Perm     os.FileMode // permission used when writing (e.g. 0600 for secrets)
	rotating bool
}

// NewFile returns a FileArtifact stored under id with the given permissions.
func NewFile(id, path string, perm os.FileMode) *FileArtifact {
	return &FileArtifact{id: id, Path: path, Perm: perm}
}

// NewRotatingFile is NewFile for a credential file whose contents a CLI silently
// refreshes/rotates in the background (e.g. a ChatGPT/OAuth token file) — the store
// keeps its stored snapshot fresh whenever its profile is about to be left, the same
// way it does for a KeychainArtifact.
func NewRotatingFile(id, path string, perm os.FileMode) *FileArtifact {
	return &FileArtifact{id: id, Path: path, Perm: perm, rotating: true}
}

// ID returns the stable, filesystem-safe name used to store this artifact.
func (f *FileArtifact) ID() string { return f.id }

// Rotates reports whether this file's contents can change independent of a profile
// switch (see NewRotatingFile).
func (f *FileArtifact) Rotates() bool { return f.rotating }

func (f *FileArtifact) Read() ([]byte, bool, error) {
	data, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (f *FileArtifact) Write(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	return AtomicWrite(f.Path, data, f.Perm)
}

// Remove deletes the file; a missing file is not an error.
func (f *FileArtifact) Remove() error {
	err := os.Remove(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// MergedFileArtifact is a FileArtifact whose top-level keys split into two kinds:
// ownedKeys (written per-profile, e.g. auth/model routing) and everything else — a
// CLI-level preference (theme, reasoning effort, ...) that should survive every
// profile switch rather than reverting to whatever a profile last captured.
type MergedFileArtifact struct {
	FileArtifact
	ownedKeys []string
	modelKey  string // top-level key holding the model id, "" if this file has none
	effortKey string // top-level key holding the reasoning-effort level, "" if none
	decode    func([]byte) (map[string]any, error)
	encode    func(map[string]any) ([]byte, error)
}

// NewMergedJSONFile is NewFile for a JSON config file that mixes profile-owned
// fields with CLI-level user preferences; only ownedKeys are swapped per profile.
func NewMergedJSONFile(id, path string, perm os.FileMode, ownedKeys ...string) *MergedFileArtifact {
	return &MergedFileArtifact{
		FileArtifact: FileArtifact{id: id, Path: path, Perm: perm},
		ownedKeys:    ownedKeys,
		decode: func(b []byte) (map[string]any, error) {
			m := map[string]any{}
			if len(b) == 0 {
				return m, nil
			}
			err := json.Unmarshal(b, &m)
			return m, err
		},
		encode: func(m map[string]any) ([]byte, error) { return json.MarshalIndent(m, "", "  ") },
	}
}

// NewMergedTOMLFile is NewMergedJSONFile for a TOML config file.
func NewMergedTOMLFile(id, path string, perm os.FileMode, ownedKeys ...string) *MergedFileArtifact {
	return &MergedFileArtifact{
		FileArtifact: FileArtifact{id: id, Path: path, Perm: perm},
		ownedKeys:    ownedKeys,
		decode: func(b []byte) (map[string]any, error) {
			m := map[string]any{}
			if len(b) == 0 {
				return m, nil
			}
			err := toml.Unmarshal(b, &m)
			return m, err
		},
		encode: func(m map[string]any) ([]byte, error) { return toml.Marshal(m) },
	}
}

// Merge returns snapshotData with every non-owned top-level key taken from liveData
// instead, so restoring a profile can't clobber a live CLI preference. Falls back to
// snapshotData unchanged if either side fails to parse, or live is empty (nothing to
// preserve yet).
func (m *MergedFileArtifact) Merge(snapshotData, liveData []byte) ([]byte, error) {
	if len(liveData) == 0 {
		return snapshotData, nil
	}
	snap, err := m.decode(snapshotData)
	if err != nil {
		return snapshotData, nil
	}
	live, err := m.decode(liveData)
	if err != nil {
		return snapshotData, nil
	}
	merged := make(map[string]any, len(live))
	for k, v := range live {
		merged[k] = v
	}
	for _, k := range m.ownedKeys {
		if v, ok := snap[k]; ok {
			merged[k] = v
		} else {
			delete(merged, k)
		}
	}
	return m.encode(merged)
}

// WithDisplay records which owned keys hold the model id and reasoning-effort level,
// so Peek can surface them for the profile list. Pass "" for a field this file
// doesn't track. Returns m for chaining onto the NewMerged*File call.
func (m *MergedFileArtifact) WithDisplay(modelKey, effortKey string) *MergedFileArtifact {
	m.modelKey, m.effortKey = modelKey, effortKey
	return m
}

// Peek decodes data (a stored snapshot's bytes) and returns its model/effort values,
// per the keys set via WithDisplay. Each is "" if untracked, unset, or unparseable.
func (m *MergedFileArtifact) Peek(data []byte) (model, effort string) {
	if m.modelKey == "" && m.effortKey == "" {
		return "", ""
	}
	decoded, err := m.decode(data)
	if err != nil {
		return "", ""
	}
	if m.modelKey != "" {
		model, _ = decoded[m.modelKey].(string)
	}
	if m.effortKey != "" {
		effort, _ = decoded[m.effortKey].(string)
	}
	return model, effort
}

// AtomicWrite writes data to path via a temp file + rename so a crash never
// leaves a half-written credential file in place.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	if perm == 0 {
		perm = 0o600
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".charon-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// KeychainArtifact is a macOS generic-password entry (e.g. Claude Code OAuth).
type KeychainArtifact struct {
	id      string
	Service string
	Account string
}

// NewKeychain returns a KeychainArtifact for the given service/account.
func NewKeychain(id, service, account string) *KeychainArtifact {
	return &KeychainArtifact{id: id, Service: service, Account: account}
}

// ID returns the stable, filesystem-safe name used to store this artifact.
func (k *KeychainArtifact) ID() string { return k.id }

// Rotates reports that a keychain entry's contents (e.g. an OAuth token) can change
// outside of any profile switch, so the store should keep its snapshot refreshed.
func (k *KeychainArtifact) Rotates() bool { return true }

func (k *KeychainArtifact) Read() ([]byte, bool, error) {
	v, err := secret.KeychainRead(k.Service)
	if errors.Is(err, secret.ErrKeychainMissing) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return []byte(v), true, nil
}

func (k *KeychainArtifact) Write(data []byte) error {
	return secret.KeychainWrite(k.Service, k.Account, string(data))
}

// Remove deletes the keychain entry; a missing entry is not an error.
func (k *KeychainArtifact) Remove() error {
	return secret.KeychainDelete(k.Service)
}
