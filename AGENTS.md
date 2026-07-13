# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository.
`charon` is a small Go CLI that detects the Codex, Claude Code, OpenCode, and Pi
CLIs and switches each one's **endpoint + credentials** between named profiles.

## Golden rule: this tool edits real user credentials

`charon` reads and writes live config for other tools (`~/.codex`, `~/.claude`,
`~/.config/opencode`, `~/.local/share/opencode`, `~/.pi/agent`) and the macOS
Keychain. It also
**reads** `~/.claude.json` (`oauthAccount.emailAddress`) solely to name an
account-backup profile — that file is never written or snapshotted.

- **Never** run `charon add`, `charon switch`, `charon save`, or the interactive menu
  against your real `$HOME` while developing. Always sandbox:
  ```sh
  HOME=$(mktemp -d) go run ./cmd/charon status
  ```
- Tests must never touch real config. Use `t.Setenv("HOME", t.TempDir())` and
  `t.Setenv("XDG_CONFIG_HOME", t.TempDir())`. See `internal/tools/tools_test.go`
  and `internal/profile/store_test.go` for the pattern.
- Do not add tests that read or write the real Keychain. The keychain shell-out
  (`internal/secret/keychain_darwin.go`) is intentionally left uncovered.
- Preserve the safety guarantees: **atomic writes** (temp file + rename),
  `0600` on credential files / `0700` on dirs, and an **auto-backup before every
  switch**. Don't regress these.

## Commands

```sh
make build      # build ./charon
make test       # go vet + go test -race ./...
make cover      # coverage summary
make lint       # golangci-lint run
make fmt        # gofmt -w .
make run        # build + open the interactive menu (sandbox your HOME first)
```

Always run `make fmt` and `make test` before finishing a change. CI
(`.github/workflows/ci.yml`) runs fmt-check, vet, `-race` tests, build, and
golangci-lint on Linux + macOS; keep all of them green.

## Architecture

```
cmd/charon/         CLI entrypoint (thin; no business logic)
  main.go           main, subcommand dispatch, usage
  commands.go       one cmd* func per subcommand + requireTool
internal/artifact/  snapshot/restore primitives, no tool knowledge
  Artifact/Rotator/Merger/Peeker interfaces; FileArtifact,
  MergedFileArtifact, KeychainArtifact; AtomicWrite
internal/tools/   per-tool adapters
  tool.go           Tool struct, AuthSpec, registry (All/Find)
  providers.go      guards for the shared "charon" provider entry (codex/opencode)
  edit.go           JSON/TOML load-merge-write helpers (preserve unknown keys)
  codex.go / claude.go / opencode.go / pi.go   one file per tool
internal/profile/ snapshot store, split by concern:
  store.go (layout/config/name validation) · snapshot.go (Save/Add/Edit/EnsureDefault)
  apply.go (Apply/Undo/Drift/refresh) · backup.go (backups + prune) · manage.go (rm/mv/cp)
internal/models/  fetch model lists from a provider API (openai/anthropic wire)
internal/secret/  masking + platform keychain (darwin vs. other build tags)
internal/tui/     bubbletea interactive menu
```

Layering (imports point left): `secret` ← `artifact` ← `tools` ← `profile` ← `cmd`/`tui`.
Profile names are validated centrally in `internal/profile/store.go` (`validateName`);
never join a user-supplied name into a path without it.

Data lives under `~/.config/charon/` (`$XDG_CONFIG_HOME` respected):
`profiles/<tool>/<name>/` (snapshot files + `manifest.json`),
`backups/<tool>/<timestamp>/`, and `config.json` (active profile per tool).

### How to add a new tool

1. Add `internal/tools/<tool>.go` returning a `*Tool` with: `Name`, `Title`,
   `Provider` (`openai`/`anthropic`), `DefaultEndpoint`, `Artifacts` (built from
   `internal/artifact` constructors), `Detected`, `Describe`, and `ApplyAuth`.
2. Register it in `All()` in `tool.go`.
3. Add a `TestXxxDescribeAndApply` in `tools_test.go` using a sandboxed `$HOME`.
   Everything else (store, CLI, TUI) is generic and needs no changes.

`ApplyAuth` must **merge** into existing config (use the `edit.go` helpers) so
unrelated user settings survive; it must not rewrite the file wholesale.

## Conventions

- Standard Go style: `gofmt`/`goimports`, tabs, error wrapping with `%w`,
  table-driven tests, small focused packages, exported identifiers documented.
- Keep `cmd/` thin — logic belongs in `internal/` packages so it stays testable.
- Never log or print full secrets; route them through `secret.Mask`.
- Prefer standard library. Current third-party deps are intentionally minimal:
  `bubbletea`/`bubbles`/`lipgloss` (TUI) and `pelletier/go-toml/v2` (Codex TOML).
  Discuss before adding more.
- Platform-specific code goes behind build tags (`_darwin.go` / `_other.go`),
  never runtime `runtime.GOOS` branching for the keychain.

## Testing expectations

- Any new behavior needs a test. Keep coverage on `internal/models`,
  `internal/profile`, and `internal/tools` from regressing.
- Use `httptest` for anything hitting the network (see `fetch_test.go`); never
  make real API calls in tests.
- The TUI is verified by compile + `go vet`; extract pure logic into testable
  helpers rather than testing the bubbletea event loop.

## Out of scope / do not do

- Don't commit built binaries, `dist/`, or coverage files (see `.gitignore`).
- Don't send config or secrets to any external service.
- Don't weaken file permissions or remove the pre-switch backup step.
