package tools

import (
	"errors"
	"os"
	"path/filepath"

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

// FileArtifact is a config or credential file owned by a tool.
type FileArtifact struct {
	id   string
	Path string
	Perm os.FileMode // permission used when writing (e.g. 0600 for secrets)
}

// NewFile returns a FileArtifact stored under id with the given permissions.
func NewFile(id, path string, perm os.FileMode) *FileArtifact {
	return &FileArtifact{id: id, Path: path, Perm: perm}
}

// ID returns the stable, filesystem-safe name used to store this artifact.
func (f *FileArtifact) ID() string { return f.id }

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
	return atomicWrite(f.Path, data, f.Perm)
}

// Remove deletes the file; a missing file is not an error.
func (f *FileArtifact) Remove() error {
	err := os.Remove(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// atomicWrite writes data to path via a temp file + rename so a crash never
// leaves a half-written credential file in place.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
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
