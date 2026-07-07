// Package tools models the AI CLIs whose endpoint + credentials charon snapshots.
package tools

import (
	"encoding/json"
	"fmt"
)

// Info is a display-friendly summary of a tool's live configuration.
type Info struct {
	Endpoint string // base URL / host in use
	AuthMode string // "oauth", "chatgpt", "api", or "none"
	Secret   string // raw secret, for masking only; never persisted here
	Model    string // active model, if known
	Account  string // logged-in account identity (email), if an OAuth login is detected; else ""
}

// withDefaults fills empty Endpoint/AuthMode with display fallbacks.
func (i Info) withDefaults(endpoint string) Info {
	if i.Endpoint == "" {
		i.Endpoint = endpoint
	}
	if i.AuthMode == "" {
		i.AuthMode = "none"
	}
	return i
}

// AuthSpec is a new endpoint + API key + model to write into a tool's config.
type AuthSpec struct {
	Endpoint string
	Key      string
	Model    string
}

// Tool describes one AI CLI's auth surface and how to summarize/reconfigure it.
type Tool struct {
	Name            string // stable id, e.g. "codex"
	Title           string // display name, e.g. "Codex"
	Artifacts       []Artifact
	Provider        string               // model-list wire format: "openai" or "anthropic"
	DefaultEndpoint string               // prefilled when adding a profile
	Detected        func() bool          // is the tool installed/configured?
	Describe        func() (Info, error) // read live config into an Info
	ApplyAuth       func(AuthSpec) error // write endpoint/key/model into live config
}

// All returns the supported tools in a stable display order.
func All() []*Tool {
	return []*Tool{newCodex(), newClaude(), newOpenCode()}
}

// ResolveEndpoint returns ep, or DefaultEndpoint when ep is empty.
func (t *Tool) ResolveEndpoint(ep string) string {
	if ep == "" {
		return t.DefaultEndpoint
	}
	return ep
}

// Find returns the tool with the given name, or nil.
func Find(name string) *Tool {
	for _, t := range All() {
		if t.Name == name {
			return t
		}
	}
	return nil
}

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
