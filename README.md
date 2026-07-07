<h1 align="center">Charon</h1>

<p align="center">
  <em>Ferry your AI tools between endpoints.</em>
</p>

<p align="center">
  <a href="https://github.com/mingtheanlay/charon/releases/latest"><img src="https://img.shields.io/github/v/release/mingtheanlay/charon?style=flat-square&color=6c47ff" alt="Latest Release"></a>
  <a href="https://github.com/mingtheanlay/charon/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/mingtheanlay/charon/ci.yml?branch=main&style=flat-square&label=CI" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/mingtheanlay/charon?style=flat-square" alt="MIT License"></a>
  <a href="https://github.com/mingtheanlay/charon/issues"><img src="https://img.shields.io/github/issues/mingtheanlay/charon?style=flat-square" alt="Open Issues"></a>
</p>

Charon is a tiny Go CLI that detects the **Codex**, **Claude Code**, and
**OpenCode** CLIs and switches each one's **endpoint + credentials** between
named profiles. Every profile is a full snapshot of that tool's auth surface, so
it works for both API-key logins and OAuth/ChatGPT sessions — and switching away
and back is always clean and reversible.

<p align="center">
  <img src="https://raw.githubusercontent.com/mingtheanlay/charon/main/assets/screenshot.png" alt="Charon interactive menu" width="80%">
</p>

## Features

- **One command, three tools.** Manage Codex, Claude Code, and OpenCode from a
  single interactive menu or a scriptable CLI.
- **Named profiles.** Snapshot each tool's full auth surface and hop between
  endpoints/keys instantly.
- **Model discovery.** Add a profile from just an endpoint + key; Charon fetches
  the model list and lets you pick one.
- **Safe by default.** Every switch is backed up first, writes are atomic, and an
  auto-captured `default` profile means you can always revert.
- **Non-destructive.** Charon only ever touches its own `charon` provider entry
  in each tool's config, never your hand-authored providers.

## Supported tools

| Tool | Endpoint | Credentials |
|------|----------|-------------|
| **Codex** | `~/.codex/config.toml` (`model_provider` → `base_url`) | `~/.codex/auth.json` |
| **Claude Code** | `~/.claude/settings.json` (`env.ANTHROPIC_BASE_URL`) | `settings.json` env key **or** macOS Keychain `Claude Code-credentials` |
| **OpenCode** | `~/.config/opencode/opencode.json` (`provider.*.options.baseURL`) | `~/.local/share/opencode/auth.json` |

## Supported platforms

| OS | Status | Notes |
|----|--------|-------|
| **macOS** (darwin) | ✅ Fully supported | Reads/writes Claude Code's OAuth token via the macOS Keychain (`security`). Primary tested platform. |
| **Linux** | ✅ Supported | File-based profiles for all tools work. Keychain access is a no-op — Claude OAuth credentials are read from `~/.claude` files instead. |
| **Windows** | ⚠️ Untested | Builds; paths resolve under `%USERPROFILE%`. Keychain is a no-op. Not yet verified. |

Keychain support is compiled in per-platform (`keychain_darwin.go` vs.
`keychain_other.go`), so non-macOS builds simply skip it.

## Installation

### curl (Linux & macOS)

No Go needed — downloads the prebuilt binary for your platform, verifies its
checksum, and installs to `~/.local/bin`:

```sh
curl -fsSL https://github.com/mingtheanlay/charon/releases/latest/download/install.sh | sh
```

> Prepend `PREFIX=/usr/local` to install system-wide, or `VERSION=v1.2.3` to pin a release.

### Homebrew (macOS & Linux)

```sh
brew install mtty80/tap/charon
```

<details>
<summary><b>Other methods</b> — manual binary · build from source</summary>

**Pre-built binary** — grab your platform's archive from the
[Releases page](https://github.com/mingtheanlay/charon/releases/latest)
(`charon_{darwin,linux}_{amd64,arm64}.tar.gz`) and verify it against the included
`checksums.txt`:

```sh
curl -L https://github.com/mingtheanlay/charon/releases/latest/download/charon_darwin_arm64.tar.gz | tar xz
sudo mv charon /usr/local/bin/
```

**From source** — requires Go 1.24+:

```sh
make install                      # build + install to ~/.local/bin (PREFIX to override)
go build -o charon ./cmd/charon   # or just build here
```

</details>

## Usage

### Interactive menu

Run `charon` with no arguments to open an arrow-key menu: pick a tool, then
switch, add, edit, or delete profiles. Quit any time with `ctrl+c`.

### CLI reference

```sh
charon                     # interactive arrow-key menu
charon status              # show each tool's active profile, endpoint, and auth
charon ls <tool>           # list saved profiles
charon save <tool> <name>  # snapshot current live config as a profile
charon models <tool>       # list models offered by an API (--key [--endpoint])
charon add <tool>          # add + activate a profile (--name --key [--endpoint --model])
charon switch <tool> <p>   # apply a saved profile (backs up current first)
charon restore <tool>      # revert to the auto-captured default
charon rm <tool> <p>       # delete a profile
```

## Adding & editing profiles

### From an endpoint + key (with model discovery)

In the menu, drill into a tool and choose **＋ Add new profile…**. The wizard:

1. asks for the **API base URL** (leave blank to accept the provider default;
   real values are never prefilled),
2. asks for the **API key** (hidden input),
3. **fetches the model list** from that endpoint (`GET /v1/models`, using
   `Authorization: Bearer` for OpenAI-style APIs and `x-api-key` for Anthropic),
4. lets you **pick a model** (or skip), then
5. names the profile — writing the endpoint/key/model into the tool's live config
   and switching to it.

### Editing an existing profile

Press **`e`** on a profile to open its edit form, showing the current **Name**,
**URL**, **Token** (masked), and **Model**. Press **`e`** on any field to change
it — selecting **Model** re-fetches the endpoint's model list so you can pick a
new one. Press **`esc`** to save your changes and switch to the profile; renaming
is handled automatically. The auto-captured **`default`** profile is protected
and cannot be edited or deleted.

### Non-interactively

```sh
charon models codex --endpoint https://openrouter.ai/api/v1 --key sk-...
charon add    codex --name openrouter --endpoint https://openrouter.ai/api/v1 \
                    --key sk-... --model openai/gpt-5.5
```

Each tool gets a dedicated `charon` provider entry written into its own config
format (Codex `[model_providers.charon]`, Claude `env.ANTHROPIC_*`, OpenCode an
`@ai-sdk/openai-compatible` provider), so switching away and back is clean.

A typical flow: log into a tool normally, `charon save codex work-key`; log into a
different endpoint/key, `charon save codex proxy`; then hop between them with
`charon switch codex work-key` — or just run `charon` and pick from the menu.
`restore` always returns to the pristine config captured the first time Charon ran.

## How it works

- **Storage:** `~/.config/charon/` (`$XDG_CONFIG_HOME` respected).
  - `profiles/<tool>/<name>/` — snapshot files + `manifest.json`.
  - `backups/<tool>/<timestamp>/` — auto-backup taken before every switch.
  - `config.json` — active profile per tool.
- **`default`** is captured automatically the first time a detected tool is seen,
  so reverting is always possible and it is never overwritten.
- Writes are **atomic** (temp file → `rename`), and credential files/dirs are
  mode `0600`/`0700`.

## Security

Profiles are stored **unencrypted** on disk (mode `0600`), including any OAuth
token copied out of the macOS Keychain. Keep `~/.config/charon` private; a future
version may push secrets back into the Keychain instead.

## Project layout

```
cmd/charon/          entrypoint + subcommands
internal/tools/      per-tool adapters (codex, claude, opencode) + artifacts
internal/profile/    snapshot store, apply, backups
internal/tui/        bubbletea interactive menu
internal/secret/     masking + macOS keychain access
```

## Development

```sh
make test    # go vet + go test -race ./...
make cover   # coverage summary
make lint    # golangci-lint run
make fmt     # gofmt -w .
```

CI (`.github/workflows/ci.yml`) runs formatting checks, vet, race tests, build,
and golangci-lint on Linux and macOS. Contributor and agent conventions —
including the rule to **always sandbox `HOME` when testing so real credentials
are never touched** — live in [AGENTS.md](AGENTS.md).

## Roadmap

- `charon undo <tool>` — restore the most recent auto-backup.
- Optional `--verify` post-switch auth ping to confirm credentials actually work.
- Windows Keychain / Credential Manager support.
- Support for more AI CLI tools.

## Contributing

**PRs and issues are very welcome.** This is an early project with plenty of room
to grow — your ideas and bug reports genuinely shape where it goes next.

- 🐛 **Found a bug?** [Open an issue](https://github.com/mingtheanlay/charon/issues/new) with the tool name, OS, and expected vs. actual behavior.
- 💡 **Have an idea?** [Start a discussion](https://github.com/mingtheanlay/charon/issues/new) — new tool support, UX tweaks, anything is fair game.
- 🔧 **Sending a fix or feature?** Fork → branch → PR. Run `make fmt && make test` before pushing. See [AGENTS.md](AGENTS.md) for the conventions.

No contribution is too small — a typo fix is as appreciated as a new feature.

## License

Released under the [MIT License](LICENSE).
