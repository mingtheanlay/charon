package tools

import (
	"encoding/json"
	"fmt"
)

// The codex and opencode adapters register their endpoint + key as a provider
// entry named "charon" inside a config block that also holds user-authored
// providers. These guards make that write refuse to touch anything but ours.

// managedProvider is the only provider entry charon owns; all others are the user's.
const managedProvider = "charon"

// snapshotProviders records the JSON of every provider but charon's, for later comparison.
func snapshotProviders(providers map[string]any) map[string]string {
	snap := map[string]string{}
	for name, v := range providers {
		if name == managedProvider {
			continue
		}
		b, _ := json.Marshal(v)
		snap[name] = string(b)
	}
	return snap
}

// ensureOnlyCharonChanged errors if the write would delete or edit any user provider.
func ensureOnlyCharonChanged(original map[string]string, providers map[string]any) error {
	for name, want := range original {
		v, ok := providers[name]
		if !ok {
			return fmt.Errorf("refusing to write config: would delete provider %q", name)
		}
		b, _ := json.Marshal(v)
		if string(b) != want {
			return fmt.Errorf("refusing to write config: would modify provider %q", name)
		}
	}
	return nil
}
