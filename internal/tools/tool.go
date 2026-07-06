// Package tools models the AI CLIs (Codex, Claude, OpenCode) whose endpoint
// and credentials charon can snapshot and switch between.
package tools

// Info is a display-friendly summary of a tool's current live configuration.
type Info struct {
	Endpoint string // base URL / host in use
	AuthMode string // "oauth", "chatgpt", "api", or "none"
	Secret   string // raw secret, used only for masking; never persisted here
	Model    string // active model, if known
}

// AuthSpec is a new endpoint + API key + model to write into a tool's config.
type AuthSpec struct {
	Endpoint string
	Key      string
	Model    string
}

// Tool describes one AI CLI: the files/keychain entries that make up its auth
// surface, plus how to summarize and reconfigure it.
type Tool struct {
	Name      string // stable id, e.g. "codex"
	Title     string // display name, e.g. "Codex"
	Artifacts []Artifact
	// Provider selects the model-list wire format ("openai" or "anthropic").
	Provider string
	// DefaultEndpoint is prefilled when adding a profile.
	DefaultEndpoint string
	// Detected reports whether the tool appears installed/configured.
	Detected func() bool
	// Describe reads the live config into an Info for display.
	Describe func() (Info, error)
	// ApplyAuth writes a new endpoint/key/model into the live config so it can
	// then be snapshotted as a profile.
	ApplyAuth func(AuthSpec) error
}

// All returns the supported tools in a stable display order.
func All() []*Tool {
	return []*Tool{newCodex(), newClaude(), newOpenCode()}
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
