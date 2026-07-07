# Charon — feature roadmap

Candidate features, ranked by value vs. effort. These lean on the existing
`Tool`/`Artifact`/`Store` abstractions, so most are additive and low-risk.

## High value, low effort

### Account-named backups ✅ (done)
`charon save <tool>` with no name snapshots the current OAuth login and names the
profile after its account (Codex `id_token` email, Claude `~/.claude.json`), so a
user with several ChatGPT/Claude accounts can capture and hop between each. In the
TUI, **`b`** backs up the highlighted profile: a login is captured under its email
(non-editable), while an API-proxy profile is duplicated to an editable `name-2`.

### More tools
The whole point of the `Tool`/`Artifact` design is cheap additions. Each new
tool is one `internal/tools/<tool>.go` returning a `*Tool` plus a
`TestXxxDescribeAndApply` case — the store, CLI, and TUI are already generic.

Targets: Gemini CLI, Aider, Cursor, Continue, Zed.

### Backup pruning ✅ (done)
Every switch/add/undo now backs up first; retention keeps the newest 10 per tool
automatically, with `charon prune <tool> [--keep N]` to trim on demand.

### `charon undo` ✅ (done)
`charon undo <tool>` reverts to the most recent backup (restoring the active
pointer too) and snapshots the current state first, so undo is itself reversible.

### Shell completions ✅ (done)
`charon completion <bash|zsh|fish>` prints a script (dynamic profile-name
completion via the hidden `charon __profiles`); goreleaser generates and bundles
them into release archives and installs them through the Homebrew formula.

## Medium

### Drift detection ✅ (done)
`Store.Drift` compares the active profile's snapshot against the live artifacts;
`status` shows `(modified)` and the TUI flags the active profile/tool with ⚠ when
the live config changed outside charon.

### `--json` output ✅ (done)
`charon status --json` and `charon ls <tool> --json` emit structured records
(secrets masked, never raw) for scripting and editor integrations.

### CLI rename / edit ✅ (done)
`charon rename <tool> <old> <new>` and `charon edit <tool> <p>
[--endpoint --key --model --name]` bring the CLI to parity with the TUI.

## Deferred / handle with care

### Profile export / import / sync
Moving profiles across machines means moving real secrets, which cuts against
the "never send secrets anywhere" guarantee (see AGENTS.md). If pursued, limit
to encrypted local export — no network sync.

---

_Context: the refactor/hygiene items (A/B) and the throttled model-fetch
loading screen are already implemented; this file tracks the remaining feature
work._
