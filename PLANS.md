# Charon — feature roadmap

Candidate features, ranked by value vs. effort. These lean on the existing
`Tool`/`Artifact`/`Store` abstractions, so most are additive and low-risk.

## High value, low effort

### More tools
The whole point of the `Tool`/`Artifact` design is cheap additions. Each new
tool is one `internal/tools/<tool>.go` returning a `*Tool` plus a
`TestXxxDescribeAndApply` case — the store, CLI, and TUI are already generic.

Targets: Gemini CLI, Aider, Cursor, Continue, Zed.

### Backup pruning
`Store.Apply` writes `backups/<tool>/<timestamp>/` on every switch and never
cleans up, so backups grow without bound. Add retention (keep last N per tool)
run automatically after a switch, plus a `charon prune` command.

### `charon undo`
Every switch already snapshots the pre-switch state under `backups/`. Expose a
one-command revert to the most recent backup — nearly free given the existing
backup machinery.

### Shell completions
Generate bash/zsh/fish completions at release time (goreleaser supports this).
Big UX win for `charon switch <tool> <TAB>` and profile-name completion.

## Medium

### Drift detection
Live config can be changed outside charon (e.g. `claude login`). Compare the
active profile's snapshot against the current live artifacts and flag
"modified since switch" in `status` and the TUI, so a stale active profile is
visible.

### `--json` output
Add machine-readable output to `status` and `ls` for scripting and editor
integrations.

### CLI rename / edit
The TUI can edit and rename profiles; add matching `charon` subcommands
(`charon edit`, `charon rename`) so the CLI reaches parity.

## Deferred / handle with care

### Profile export / import / sync
Moving profiles across machines means moving real secrets, which cuts against
the "never send secrets anywhere" guarantee (see AGENTS.md). If pursued, limit
to encrypted local export — no network sync.

---

_Context: the refactor/hygiene items (A/B) and the throttled model-fetch
loading screen are already implemented; this file tracks the remaining feature
work._
