package tools

import (
	"encoding/json"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// loadJSONMap reads path as a JSON object, returning an empty map if absent.
func loadJSONMap(path string) (map[string]any, error) {
	m := map[string]any{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// writeJSONMap writes m to path (0600) as indented JSON, atomically.
func writeJSONMap(path string, m map[string]any, perm os.FileMode) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), perm)
}

// loadTOMLMap reads path as a TOML table, returning an empty map if absent.
func loadTOMLMap(path string) (map[string]any, error) {
	m := map[string]any{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// writeTOMLMap writes m to path (0644) as TOML, atomically.
func writeTOMLMap(path string, m map[string]any, perm os.FileMode) error {
	data, err := toml.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicWrite(path, data, perm)
}

// subMap returns m[key] as a map, creating it if missing.
func subMap(m map[string]any, key string) map[string]any {
	if existing, ok := m[key].(map[string]any); ok {
		return existing
	}
	created := map[string]any{}
	m[key] = created
	return created
}
