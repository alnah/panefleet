# panefleet

> tmux workboard plugin for agent panes. Provides a popup board, pane preview, theme picker, and state detection for Codex, Claude Code, OpenCode, and shell panes.

Panefleet started as a way to reduce context switching across tmux windows and sessions while running Codex, Claude Code, and OpenCode in parallel. The agents can produce strong code, but I still need to track the production chain around them: implementation, behavior checks, security, testability, refactors, optimization, and portability across several chat sessions and projects at once.

The useful part is not only faster navigation. It is seeing the worker states in one place: `RUN`, `DONE`, `IDLE`, `STALE`, `WAIT`, and the rest. When several workers are active, it is easy to forget that one pane is waiting for approval, that another one finished, or that a third one has gone stale. Keeping those states visible reduces the cognitive load of orchestrating the work and makes parallel sessions much easier to manage.

If better hooks become available later, or if bridge distribution gets simpler, panefleet can use them. For now, it stays focused on a practical tmux workboard with portable defaults and known limitations.

## Table of contents

- [Installation](#installation)
- [Requirements](#requirements)
- [Features](#features)
- [Configuration](#configuration)
- [Status model](#status-model)
- [Optional integrations](#optional-integrations)
- [Observability](#observability)
- [Troubleshooting](#troubleshooting)
- [Testing](#testing)

## Installation

Clone the repo, then use the Makefile:

```bash
git clone https://github.com/alnah/panefleet.git
cd panefleet
make install core      # core only, heuristic-first
make install codex     # core + codex
make install claude    # core + claude
make install opencode  # core + opencode
make install all       # core + codex + claude + opencode
```

`make install core` is the heuristic-only path. It works without integrations, but status detection is less reliable than the provider-assisted modes.

`make install codex`, `make install claude`, `make install opencode`, and `make install all` can automatically download a prebuilt `panefleet-agent-bridge` binary from this repo's GitHub Releases.

`make install ...` first checks the core system dependencies and installs missing ones through the detected package manager. That step is explicit in the command output and may prompt for `sudo` on Linux.

Outside tmux, `make install ...` then runs preflight checks and prints the exact `tmux source-file ...` command to load the plugin from the checkout. Inside tmux, it also installs the local bindings directly.

The install targets also bind:

- `prefix + P` for the board
- `prefix + T` for the theme picker

## Requirements

Core runtime:

- `tmux` with `display-popup`
- `fzf` with `--header-lines-border`
- `ripgrep`
- `bash`

Optional runtime:

- `curl` and `tar` to download a prebuilt bridge from GitHub Releases
- `bun` and the OpenCode plugin host only for the OpenCode plugin integration

`make install ...` handles missing core dependencies automatically through the detected package manager. Run `make doctor` inside tmux to inspect the installed state:

```bash
make doctor
```

## Features

- Popup board built on `tmux` and `fzf`
- Heuristics-first pane states for Codex, Claude Code, OpenCode, and shell panes
- Pane jump, preview, theme preview, and theme apply commands
- Install, uninstall, and install diagnostics commands
- Simple `make install <provider>` workflow
- `make doctor` and `make uninstall`
- State inspection with `state-show`, `state-list`, and `doctor --verbose`
- Theme palettes with truecolor, 256-color, and ANSI fallback
- Optional adapter bridge kept outside the default install path

## Configuration

Panefleet uses tmux global options.

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

Board and preview behavior:

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

## Optional integrations

Panefleet works without any adapter bridge. That remains the default and recommended install path.

All provider integrations reuse the same `panefleet-agent-bridge` binary. Provider-specific setup only adds the matching wrapper or plugin layer on top of that shared bridge.

Install integrations explicitly by provider:

```bash
make install codex
make install claude
make install opencode
make install all
```

All provider installs:

- install the shared `panefleet-agent-bridge`
- switch `@panefleet-adapter-mode` to `auto` when run inside tmux
- keep the core plugin usable even without integrations

The bridge resolves in this order:

1. exact GitHub Release asset for the current checkout tag, when the checkout is on a release tag
2. local source build when `go` is available
3. latest matching GitHub Release asset from this repo for the current OS/arch

The provider install commands use that order automatically. So, in the normal case, you do not need Go installed just to use provider integrations. A local Go toolchain is only the fallback path when no matching prebuilt bridge can be downloaded.

Provider-specific behavior:

- `make install codex`
  - keeps live Codex heuristics available for `RUN` and `WAIT`
  - uses bridge events when available
- `make install claude`
  - installs and uses the Claude hook wrapper
- `make install opencode`
  - writes `~/.config/opencode/plugins/panefleet.ts` by default
  - still requires `bun` and the OpenCode plugin host
- `make install all`
  - runs the three provider installs above

Important constraints:

- missing bridge errors are explicit
- the core plugin still works when no integration is installed, but heuristic-only status detection is less reliable than bridge-assisted mode
- OpenCode plugin integration requires its plugin host and `bun`
- these integrations are optional and do not change the default tmux install

## Observability

Useful commands:

```bash
make doctor
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

Maintainer release helpers:

```bash
make bridge
make bridge-download
make release-check
```
