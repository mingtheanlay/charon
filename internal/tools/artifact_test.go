package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileArtifactRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "secret.json")
	a := NewFile("secret.json", path, 0o600)

	if _, exists, err := a.Read(); err != nil || exists {
		t.Fatalf("expected absent artifact, exists=%v err=%v", exists, err)
	}

	if err := a.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, exists, err := a.Read()
	if err != nil || !exists || string(data) != "hello" {
		t.Fatalf("Read after write: data=%q exists=%v err=%v", data, exists, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %v, want 0600", info.Mode().Perm())
	}

	if err := a.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, exists, _ := a.Read(); exists {
		t.Error("artifact still exists after Remove")
	}
	// Removing a missing artifact is not an error.
	if err := a.Remove(); err != nil {
		t.Errorf("Remove(absent): %v", err)
	}
}

func TestAtomicWriteReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := atomicWrite(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "v2" {
		t.Errorf("got %q, want v2", data)
	}
}
