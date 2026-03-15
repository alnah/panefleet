# panefleet

> tmux workboard plugin for agent panes. Provides a popup board, pane preview, theme picker, and state detection for Codex, Claude Code, OpenCode, and shell panes.

[TODO: GIF]

Panefleet started as a way to reduce context switching across tmux windows and sessions while running Codex, Claude Code, and OpenCode in parallel. The agents can produce strong code, but I still need to track the production chain around them: implementation, behavior checks, security, testability, refactors, optimization, and portability across several chat sessions and projects at once.

The useful part is not only faster navigation. It is seeing the worker states in one place: `RUN`, `DONE`, `IDLE`, `STALE`, `WAIT`, and the rest. When several workers are active, it is easy to forget that one pane is waiting for approval, that another one finished, or that a third one has gone stale. Keeping those states visible reduces the cognitive load of orchestrating the work and makes parallel sessions much easier to manage.

If better hooks become available later, or if bridge distribution gets simpler, panefleet can use them. For now, it stays focused on a practical tmux workboard with portable defaults and known limitations.

## Table of contents

- [Installation](#installation)
- [Features](#features)
- [Usage](#usage)
- [Configuration](#configuration)
- [Status model](#status-model)
- [Observability](#observability)
- [Troubleshooting](#troubleshooting)
- [Testing](#testing)

## Installation

Preferred install path (Homebrew):

```bash
brew tap alnah/tap
brew install alnah/tap/panefleet
panefleet install all
panefleet doctor --install
```

Install targets:
- `core`: heuristic-only runtime (no bridge)
- `codex`: core + Codex integration
- `claude`: core + Claude integration
- `opencode`: core + OpenCode integration
- `all`: core + Codex + Claude + OpenCode

`panefleet install` without a target defaults to `core`.

<details>
<summary>Source install (clone + make)</summary>

```bash
git clone https://github.com/alnah/panefleet.git
cd panefleet
make install all
make doctor
```

</details>

What install does:
- checks and installs missing runtime dependencies with the detected package manager
- installs tmux bindings (`prefix + P`, `prefix + T`)
- for provider targets, downloads a prebuilt Go bridge from GitHub Releases when available
- falls back to local bridge build only if release download is unavailable and Go is present

Install output contract:
- `install core` prints `Core installed` and keeps adapter mode `heuristic-only`
- provider installs print integration result + `Bridge: ...` + `Adapter mode: auto`
- re-running `install all` is idempotent and reports `Bridge: already installed`
- force bridge refresh is available with `PANEFLEET_BRIDGE_INSTALL_MODE=force-build` or `force-download`

## Features

- Popup board built on `tmux` and `fzf`
- Pane states for Codex, Claude Code, OpenCode, and shell panes
- State inspection with `state-show`, `state-list`, and `doctor --verbose`
- Theme palettes with truecolor, 256-color, and ANSI fallback

## Usage

Board behavior:

- Open the board with `prefix + P`.
- Open the theme picker with `prefix + T`.
- The board list is sorted first by state priority, then by recent activity.
- `enter` jumps to the selected pane.
- `up` and `down` move the selection in the list.
- The preview pane shows the selected pane metadata plus as many visible trailing lines as fit.
- `ctrl-r` reloads the board content without leaving the popup.

## Status model

Panefleet displays these states:

| State   | Meaning                                                      |
| ------- | ------------------------------------------------------------ |
| `RUN`   | Active work in progress                                      |
| `WAIT`  | Clear chooser or approval prompt                             |
| `DONE`  | Work appears finished and the pane is back at a ready prompt |
| `IDLE`  | No strong sign of active work                                |
| `STALE` | Left open beyond the configured stale threshold              |
| `ERROR` | Dead pane with a non-zero exit status                        |

Status resolution order:

1. Manual override
2. Fresh adapter state when `@panefleet-adapter-mode=auto`
3. Provider heuristics
4. Generic shell and dead-process fallback

Provider heuristics are intentionally narrow:

- `Codex`
  - `WAIT` from visible choosers and approval prompts
  - `RUN` from process-tree activity and visible work markers
  - `DONE` from the prompt returning
- `Claude Code`
  - `WAIT` from visible chooser or approval prompts
  - `RUN` from visible active work markers
  - `DONE` from a ready prompt returning
- `OpenCode`
  - `WAIT` from visible chooser or approval prompts
  - `RUN` from visible activity markers in the active pane area
  - `DONE` from the ready footer and prompt area

`WAIT` is less reliable than the other states. Expect the most stable results from `RUN`, `DONE`, `IDLE`, and `STALE`.

## Configuration

Panefleet uses tmux global options. Use them to tweak your installation. Here are my defaults:

```tmux
set -g @panefleet-theme panefleet-dark
set -g @panefleet-done-recent-minutes 10
set -g @panefleet-stale-minutes 45
set -g @panefleet-agent-status-max-age-seconds 600
set -g @panefleet-adapter-mode heuristic-only
```

Supported options:

| Option                                    | Default          | Description                                            |
| ----------------------------------------- | ---------------- | ------------------------------------------------------ |
| `@panefleet-theme`                        | `panefleet-dark` | Active board theme                                     |
| `@panefleet-done-recent-minutes`          | `10`             | How long `DONE` stays visible before aging into `IDLE` |
| `@panefleet-stale-minutes`                | `45`             | When `IDLE` ages into `STALE`                          |
| `@panefleet-agent-status-max-age-seconds` | `600`            | Freshness window for adapter-provided states           |
| `@panefleet-adapter-mode`                 | `heuristic-only` | `heuristic-only` or `auto`                             |

Color portability:

- automatic fallback: truecolor > 256 colors > ANSI
- optional override: `PANEFLEET_COLOR_MODE=truecolor|256|ansi`

## Observability

Useful commands:

```bash
make doctor
bin/panefleet doctor --verbose
```

What they expose:

- final displayed state, raw state, source, and reason
- adapter freshness and timestamps
- cached heuristic signature and live signature
- install status, bindings, and hook counts
- state counts across all panes

Optional logs:

- `PANEFLEET_RUNTIME_LOG_DIR` for runtime events
- `PANEFLEET_EVENT_LOG_DIR` for adapter bridge payload and decision logs

Example:

```bash
export PANEFLEET_RUNTIME_LOG_DIR=~/.local/state/panefleet/runtime
export PANEFLEET_EVENT_LOG_DIR=~/.local/state/panefleet/events
bin/panefleet doctor --verbose
tail -n +1 ~/.local/state/panefleet/runtime/runtime.log
```

## Troubleshooting

Start with:

```bash
make doctor
```

Common checks:

- `preflight` fails
  - verify `tmux`, `fzf`, and `rg` are installed
  - verify `tmux` supports `display-popup`
  - verify `fzf` supports `--header-lines-border`
- `doctor --install` shows `bridge-missing`
  - run `make install codex`, `make install claude`, `make install opencode`, or `make install all`
  - if release download is unavailable, install `go` and rerun
- board does not open
  - reload `panefleet.tmux`
  - run `make doctor`
  - verify `prefix + P` is bound
- status looks wrong for one pane
  - run `bin/panefleet state-show --pane %pane`
  - inspect `final.source` and `final.reason`
- OpenCode integration is not active
  - verify `bun` is installed
  - verify `make doctor` points to the expected `opencode.plugin`
  - rerun `make install opencode`

If you installed panefleet with Homebrew, uninstall it with:

```bash
brew uninstall panefleet
# optional: remove the tap if you no longer use it
brew untap alnah/tap
```

Reset the plugin bindings and hooks:

```bash
make uninstall
```

Use `make uninstall` to remove the tmux bindings and hooks installed by panefleet.

## Testing

Run the full local regression suite with:

```bash
./scripts/test.sh
make test
```

That runs:

- `go test ./...`
- `go test -race ./cmd/panefleet-agent-bridge`
- `shellcheck`
- the shell regression harness in `tests/test_panefleet.sh`
- the make-install contract harness in `tests/test_make_install.sh`

Maintainer release helpers:

```bash
make bridge
make bridge-download
make release-check
```

Release readiness checklist (maintainers):

1. Run `./scripts/test.sh`.
2. Run `make release-check`.
3. Confirm install contract manually in tmux with one target (`make install all`) then `make doctor`.
