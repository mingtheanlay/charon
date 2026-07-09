// Package profile stores and applies named snapshots of each tool's auth surface,
// with automatic backups and an always-available "default".
//
// The package is split across a few files:
//   - store.go     the Store, its on-disk layout, config, and name validation
//   - snapshot.go  capturing profiles: Save / AddProfile / EditProfile / EnsureDefault
//   - apply.go     restoring profiles: Apply / Undo / refresh / Drift
//   - backup.go    timestamped pre-switch backups and pruning
//   - manage.go    Remove / Rename / Duplicate
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charon/internal/artifact"
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
	return artifact.AtomicWrite(s.configPath(), data, 0o600)
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

// writeManifest writes a profile/backup dir's manifest.json atomically.
func writeManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return artifact.AtomicWrite(filepath.Join(dir, "manifest.json"), data, 0o600)
}

// ProfileModelEffort reads a stored profile's own config snapshot and returns its
// captured model/effort — e.g. so the profile list can show each account's own
// model and reasoning-effort level, not just whatever is live right now. Both are ""
// if the tool's config artifact doesn't track them or the profile has no record.
func (s *Store) ProfileModelEffort(t *tools.Tool, name string) (model, effort string) {
	if !s.Exists(t.Name, name) {
		return "", ""
	}
	dir := s.profDir(t.Name, name)

	if t.Name == "opencode" {
		cfgPath := filepath.Join(dir, "opencode.jsonc")
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			cfgPath = filepath.Join(dir, "opencode.json")
		}
		if data, err := os.ReadFile(cfgPath); err == nil {
			var cfg struct {
				Model           string `json:"model"`
				SmallModel      string `json:"small_model"`
				ReasoningEffort string `json:"reasoningEffort"`
				Agent           map[string]struct {
					Model           string `json:"model"`
					ReasoningEffort string `json:"reasoningEffort"`
				} `json:"agent"`
				Agents map[string]struct {
					Model           string `json:"model"`
					ReasoningEffort string `json:"reasoningEffort"`
				} `json:"agents"`
			}
			if json.Unmarshal(data, &cfg) == nil {
				model = strings.TrimPrefix(cfg.Model, "charon/")
				if model == "" {
					model = strings.TrimPrefix(cfg.SmallModel, "charon/")
				}
				effort = cfg.ReasoningEffort

				// Fallback to agent-specific configs
				if model == "" {
					for _, agent := range cfg.Agents {
						if agent.Model != "" {
							model = strings.TrimPrefix(agent.Model, "charon/")
							break
						}
					}
				}
				if model == "" && cfg.Agent != nil {
					for _, agent := range cfg.Agent {
						if agent.Model != "" {
							model = strings.TrimPrefix(agent.Model, "charon/")
							break
						}
					}
				}

				if effort == "" {
					for _, agent := range cfg.Agents {
						if agent.ReasoningEffort != "" {
							effort = agent.ReasoningEffort
							break
						}
					}
				}
				if effort == "" && cfg.Agent != nil {
					for _, agent := range cfg.Agent {
						if agent.ReasoningEffort != "" {
							effort = agent.ReasoningEffort
							break
						}
					}
				}
				return model, effort
			}
		}
	}

	for _, a := range t.Artifacts {
		peeker, ok := a.(artifact.Peeker)
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, a.ID()))
		if err != nil {
			continue
		}
		return peeker.Peek(data)
	}
	return "", ""
}

// validateName rejects a profile name the store cannot safely use as a directory
// name under its root: empty, "." / ".." (which would escape or alias the profiles
// dir), or anything sanitizeProfileName would alter (path separators, spaces, ...).
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	if name == "." || name == ".." || name != sanitizeProfileName(name) {
		return fmt.Errorf("invalid profile name %q (use letters, digits, and . _ @ -)", name)
	}
	return nil
}

// validateNewName is validateName for a profile being created or renamed, where
// the reserved default name is additionally off limits.
func validateNewName(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if name == DefaultName {
		return fmt.Errorf("%q is a reserved name", DefaultName)
	}
	return nil
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
