// Package tools models the AI CLIs whose endpoint + credentials charon snapshots.
package tools

import (
	"os"
	"os/exec"

	"charon/internal/artifact"
)

// Info is a display-friendly summary of a tool's live configuration.
type Info struct {
	Endpoint string // base URL / host in use
	AuthMode string // "oauth", "chatgpt", "api", or "none"
	Secret   string // raw secret, for masking only; never persisted here
	Model    string // active model, if known
	Effort   string // active reasoning-effort level, if known
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
	Endpoint  string
	Key       string
	Model     string
	AllModels []string // full fetched model list (e.g. TUI wizard's picker results); "" entries not included
}

// Tool describes one AI CLI's auth surface and how to summarize/reconfigure it.
type Tool struct {
	Name             string // stable id, e.g. "codex"
	Title            string // display name, e.g. "Codex"
	Artifacts        []artifact.Artifact
	Provider         string               // model-list wire format: "openai" or "anthropic"
	DefaultEndpoint  string               // prefilled when adding a profile
	Detected         func() bool          // is the tool installed/configured?
	Describe         func() (Info, error) // read live config into an Info
	ApplyAuth        func(AuthSpec) error // write endpoint/key/model into live config
	OfficialOAuth    func() bool          // official OAuth credentials exist despite custom routing
	UseOfficialAuth  func() error         // clear custom routing without removing OAuth credentials
	OAuthFingerprint func() string        // identifies the current OAuth credential (e.g. its mtime); "" if none. Used to detect a fresh login versus a long-standing token.
}

func detected(executable string, paths ...string) bool {
	if _, err := exec.LookPath(executable); err == nil {
		return true
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// All returns the supported tools in a stable display order.
func All() []*Tool {
	return []*Tool{newCodex(), newClaude(), newOpenCode(), newPi()}
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
