<p align="center">
  <img src="assets/banner.png" alt="Charon" width="100%">
</p>

<p align="center">
  <a href="https://github.com/mingtheanlay/charon/releases/latest"><img src="https://img.shields.io/github/v/release/mingtheanlay/charon?style=flat-square&color=6c47ff" alt="Latest Release"></a>
  <a href="https://github.com/mingtheanlay/charon/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/mingtheanlay/charon/ci.yml?branch=main&style=flat-square&label=CI" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/mingtheanlay/charon?style=flat-square" alt="MIT License"></a>
  <a href="https://github.com/mingtheanlay/charon/issues"><img src="https://img.shields.io/github/issues/mingtheanlay/charon?style=flat-square" alt="Open Issues"></a>
</p>

# Charon

A tiny Go CLI that detects the **Codex**, **Claude Code**, and **OpenCode**
CLIs and switches each one's **endpoint + credentials** between named profiles.
Each profile is a full snapshot of that tool's auth surface, so it works for
both API-key logins and OAuth/ChatGPT sessions.

Running `charon` opens an interactive menu behind this banner:

```
 ██████╗██╗  ██╗ █████╗ ██████╗  ██████╗ ███╗   ██╗
██╔════╝██║  ██║██╔══██╗██╔══██╗██╔═══██╗████╗  ██║
██║     ███████║███████║██████╔╝██║   ██║██╔██╗ ██║
██║     ██╔══██║██╔══██║██╔══██╗██║   ██║██║╚██╗██║
╚██████╗██║  ██║██║  ██║██║  ██║╚██████╔╝██║ ╚████║
 ╚═════╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═══╝
 ⛴  ferry your AI tools between endpoints · q to quit
```

## What it manages

| Tool | Endpoint | Credentials |
|------|----------|-------------|
| Codex | `~/.codex/config.toml` (`model_provider` → `base_url`) | `~/.codex/auth.json` |
| Claude Code | `~/.claude/settings.json` (`env.ANTHROPIC_BASE_URL`) | `settings.json` env key **or** macOS Keychain `Claude Code-credentials` |
| OpenCode | `~/.config/opencode/opencode.json` (`provider.*.options.baseURL`) | `~/.local/share/opencode/auth.json` |

## Supported operating systems

| OS | Status | Notes |
|----|--------|-------|
| **macOS** (darwin) | ✅ Fully supported | Reads/writes Claude Code's OAuth token via the macOS Keychain (`security`). Primary tested platform. |
| **Linux** | ✅ Supported | File-based profiles for all tools work. Claude Keychain access is a no-op — Claude OAuth credentials are picked up from `~/.claude` files instead. |
| **Windows** | ⚠️ Untested | Builds; file paths resolve under `%USERPROFILE%`. Keychain is a no-op. Not yet verified. |

Keychain support is compiled in per-platform (`keychain_darwin.go` vs.
`keychain_other.go`), so non-macOS builds simply skip it.

---

## Install

### Option 1 — Homebrew (macOS & Linux, recommended)

```sh
brew tap mtty80/tap
brew install charon
```

To upgrade later:

```sh
brew upgrade charon
```

### Option 2 — Download a pre-built binary

Grab the latest binary for your platform from the
[**Releases page**](https://github.com/mingtheanlay/charon/releases/latest):

| Platform | Archive |
|----------|---------|
| macOS (Apple Silicon) | `charon_darwin_arm64.tar.gz` |
| macOS (Intel) | `charon_darwin_amd64.tar.gz` |
| Linux (x86-64) | `charon_linux_amd64.tar.gz` |
| Linux (ARM64) | `charon_linux_arm64.tar.gz` |

```sh
# Example for macOS Apple Silicon
curl -L https://github.com/mingtheanlay/charon/releases/latest/download/charon_darwin_arm64.tar.gz | tar xz
sudo mv charon /usr/local/bin/
```

Each release includes a `checksums.txt` — verify with:

```sh
shasum -a 256 -c checksums.txt
```

### Option 3 — Build from source

Requires **Go 1.24+**.

```sh
# Quick install to ~/.local/bin (no sudo)
./install.sh

# Or choose a custom prefix
PREFIX=/usr/local ./install.sh

# Or use the Makefile
make install          # build + install to ~/.local/bin
make build            # just build ./charon here

# Or plain go build
go build -o charon ./cmd/charon
```

---

## Use

```sh
charon                     # interactive arrow-key menu
charon status              # show each tool's active profile, endpoint, auth
charon ls <tool>           # list saved profiles
charon save <tool> <name>  # snapshot current live config as a profile
charon models <tool>       # list models offered by an API (--key [--endpoint])
charon add <tool>          # add+activate a profile (--name --key [--endpoint --model])
charon switch <tool> <p>   # apply a saved profile (backs up current first)
charon restore <tool>      # revert to the auto-captured original
charon rm <tool> <p>       # delete a profile
```

## Add a profile from an endpoint + key (with model discovery)

In the menu, drill into a tool and pick **＋ Add new profile…**. The wizard:

1. asks for the **API base URL** (shown as a placeholder — leave blank to accept
   the provider default; real values are never prefilled),
2. asks for the **API key** (hidden input),
3. **fetches the model list** from that endpoint (`GET /v1/models`, using
   `Authorization: Bearer` for OpenAI-style and `x-api-key` for Anthropic),
4. lets you **pick a model** (or skip),
5. names the profile — then writes the endpoint/key/model into the tool's live
   config and switches to it.

Press **`e`** on an existing profile to open an **edit form** with its current
**Name**, **URL**, **Token** (masked), and **Model**. Select any field to change
it; selecting **Model** re-fetches the endpoint's model list so you can pick a
new one. Choose **✓ Save changes** to apply (renaming is handled automatically).

From then on it's just another profile you can `switch` between. The same flow
non-interactively:

```sh
charon models codex --endpoint https://openrouter.ai/api/v1 --key sk-...
charon add    codex --name openrouter --endpoint https://openrouter.ai/api/v1 \
                  --key sk-... --model openai/gpt-5.5
```

Each tool gets a dedicated `charon` provider entry written into its own config
format (Codex `[model_providers.charon]`, Claude `env.ANTHROPIC_*`, OpenCode an
`@ai-sdk/openai-compatible` provider), so switching away and back is clean.

Typical flow: log into a tool normally, `charon save codex work-key`, log into a
different endpoint/key, `charon save codex proxy`, then hop between them with
`charon switch codex work-key` — or just run `charon` and pick from the menu.
`restore` always returns to the pristine config captured the first time charon
ran.

---

## How it works

- **Storage:** `~/.config/charon/` (`$XDG_CONFIG_HOME` respected).
  - `profiles/<tool>/<name>/` — snapshot files + `manifest.json`.
  - `backups/<tool>/<timestamp>/` — auto-backup taken before every switch.
  - `config.json` — active profile per tool.
- **`original`** is captured automatically the first time a detected tool is
  seen, so reverting is always possible and never overwritten.
- Writes are **atomic** (temp file → `rename`) and credential files/dirs are
  `0600`/`0700`.

## Security note

Profiles are stored **unencrypted** on disk (mode 0600), including any OAuth
token copied out of the macOS Keychain. Keep `~/.config/charon` private; a future
version can push secrets back into the Keychain instead.

---

## Layout

```
cmd/charon/            entrypoint + subcommands
internal/tools/      per-tool adapters (codex, claude, opencode) + artifacts
internal/profile/    snapshot store, apply, backups
internal/tui/        bubbletea interactive menu
internal/secret/     masking + macOS keychain access
```

---

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

---

## Contributing & Feedback

**PRs and issues are very welcome.** This is an early project and there's a lot
of room to grow — your ideas and bug reports genuinely shape where it goes next.

### Ways to help

- 🐛 **Found a bug?** [Open an issue](https://github.com/mingtheanlay/charon/issues/new) with the tool name, OS, and what you expected vs. what happened.
- 💡 **Have an idea?** [Start a discussion](https://github.com/mingtheanlay/charon/issues/new) — new tool support, UX tweaks, anything is fair game.
- 🔧 **Want to send a fix or feature?** Fork → branch → PR. Run `make fmt && make test` before pushing. See [AGENTS.md](AGENTS.md) for the conventions.

No contribution is too small — a typo fix or a one-line doc improvement is just as appreciated as a new feature.

### Planned next steps

- `charon undo <tool>` — restore the most recent auto-backup.
- Optional `--verify` post-switch auth ping to confirm the credentials actually work.
- Windows Keychain / Credential Manager support.
- Support for more AI CLI tools.

If any of these matter to you, comment on the relevant issue or open a new one — it helps prioritise.
