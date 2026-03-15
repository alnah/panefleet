# panefleet

> tmux workboard plugin for agent panes. Provides a popup board, pane preview, theme picker, and heuristics-first state detection for Codex, Claude Code, OpenCode, and shell panes.

## Table of contents

- [Installation](#installation)
- [Requirements](#requirements)
- [Quick start](#quick-start)
- [Features](#features)
- [Configuration](#configuration)
- [CLI reference](#cli-reference)
- [Status model](#status-model)
- [Optional integrations](#optional-integrations)
- [Observability](#observability)
- [Troubleshooting](#troubleshooting)
- [Testing](#testing)

## Installation

### Source checkout

```bash
git clone https://github.com/alnah/panefleet.git ~/workspace/panefleet
cd ~/workspace/panefleet
./scripts/install-deps-homebrew.sh
tmux source-file "$PWD/panefleet.tmux"
```

That loads the plugin directly from the checkout. No symlink is required.

### TPM-style path

```bash
mkdir -p ~/.tmux/plugins
ln -sfn "$PWD" ~/.tmux/plugins/panefleet
tmux source-file ~/.tmux/plugins/panefleet/panefleet.tmux
```

### Local lifecycle commands

```bash
bin/panefleet install
bin/panefleet install-integrations
bin/panefleet reconcile
bin/panefleet uninstall
bin/panefleet doctor --install
```

`install` and `reconcile` bind:

- `prefix + P` for the board
- `prefix + T` for the theme picker

<details>
<summary>Optional build dependency</summary>

The heuristics-first core does not require Go.

Build the optional adapter bridge only if you want adapter integrations:

```bash
./scripts/install-deps-homebrew.sh --with-go
bin/panefleet install-integrations
```

Inside tmux, `install-integrations` also switches `@panefleet-adapter-mode` to `auto`.

</details>

## Requirements

Core runtime:

- `tmux` with `display-popup`
- `fzf` with `--header-lines-border`
- `ripgrep`
- `bash`

Optional runtime:

- `go` to build `bin/panefleet-agent-bridge` from source
- `bun` and the OpenCode plugin host only for the OpenCode plugin integration

Check the local runtime with:

```bash
bin/panefleet preflight
```

## Quick start

```bash
bin/panefleet preflight
tmux source-file ./panefleet.tmux
bin/panefleet popup
bin/panefleet doctor --verbose
```

Useful tmux actions:

- `prefix + P` opens the board
- `prefix + T` opens the theme picker
- `enter` jumps to the selected pane
- `up` and `down` navigate the list
- `ctrl-r` reloads the list

## Features

- Popup board built on `tmux` and `fzf`
- Heuristics-first pane states for Codex, Claude Code, OpenCode, and shell panes
- Pane jump, preview, theme preview, and theme apply commands
- Install, reconcile, uninstall, and install diagnostics commands
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

| Option | Default | Description |
| --- | --- | --- |
| `@panefleet-theme` | `panefleet-dark` | Active board theme |
| `@panefleet-done-recent-minutes` | `10` | How long `DONE` stays visible before aging into `IDLE` |
| `@panefleet-stale-minutes` | `45` | When `IDLE` ages into `STALE` |
| `@panefleet-agent-status-max-age-seconds` | `600` | Freshness window for adapter-provided states |
| `@panefleet-adapter-mode` | `heuristic-only` | `heuristic-only` or `auto` |

Color portability:

- automatic fallback: truecolor -> 256 colors -> ANSI
- optional override: `PANEFLEET_COLOR_MODE=truecolor|256|ansi`

## CLI reference

```bash
bin/panefleet popup
bin/panefleet board
bin/panefleet list
bin/panefleet preview %1
bin/panefleet jump %1

bin/panefleet install
bin/panefleet install-integrations
bin/panefleet reconcile
bin/panefleet uninstall

bin/panefleet preflight
bin/panefleet doctor --install
bin/panefleet doctor --verbose

bin/panefleet state-show --pane %1
bin/panefleet inspect --pane %1
bin/panefleet state-list
bin/panefleet state-set --pane %1 --status RUN --tool codex --source test
bin/panefleet state-clear --pane %1

bin/panefleet themes
bin/panefleet theme-select
bin/panefleet theme-popup
bin/panefleet theme-preview dracula
bin/panefleet theme-apply dracula
```

<details>
<summary>Board and preview behavior</summary>

- The list is sorted by state priority and activity recency.
- The preview shows pane metadata plus the visible tail of the pane.
- `up` and `down` move in the list.
- `ctrl-r` reloads the list.

</details>

## Status model

Panefleet displays these states:

| State | Meaning |
| --- | --- |
| `RUN` | Active work in progress |
| `WAIT` | Clear chooser or approval prompt |
| `DONE` | Work appears finished and the pane is back at a ready prompt |
| `IDLE` | No strong sign of active work |
| `STALE` | Left open beyond the configured stale threshold |
| `ERROR` | Dead pane with a non-zero exit status |

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

Panefleet works without any adapter bridge. That is the default install path.

Optional integration files in this repo:

- `scripts/claude-code-hook`
- `scripts/codex-app-server-bridge`
- `scripts/codex-notify-bridge`
- `scripts/opencode-event-bridge`
- `scripts/opencode-panefleet.ts`
- `bin/panefleet-agent-bridge`

Build the bridge explicitly:

```bash
bin/panefleet install-integrations
```

If the command is run inside tmux, it also enables adapter mode for that session.

Important constraints:

- wrappers do not auto-build the bridge
- missing bridge errors are explicit
- OpenCode plugin integration requires its plugin host and `bun`
- these integrations are optional and do not change the default tmux install

## Observability

Useful commands:

```bash
bin/panefleet state-show --pane %1
bin/panefleet inspect --pane %1
bin/panefleet state-list
bin/panefleet doctor --verbose
bin/panefleet doctor --install
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
bin/panefleet preflight
bin/panefleet doctor --install
bin/panefleet doctor --verbose
```

Common checks:

- `preflight` fails
  - verify `tmux`, `fzf`, and `rg` are installed
  - verify `tmux` supports `display-popup`
  - verify `fzf` supports `--header-lines-border`
- board does not open
  - reload `panefleet.tmux`
  - run `bin/panefleet doctor --install`
  - verify `prefix + P` is bound
- status looks wrong for one pane
  - run `bin/panefleet state-show --pane %pane`
  - inspect `final.source` and `final.reason`
- optional bridge wrapper fails
  - build `bin/panefleet-agent-bridge` explicitly with `bin/panefleet install-integrations`

Reset the plugin bindings and hooks:

```bash
bin/panefleet uninstall
bin/panefleet install
```

## Testing

Run the full local regression suite with:

```bash
./scripts/test.sh
```

That runs:

- `go test ./...`
- `go test -race ./cmd/panefleet-agent-bridge`
- `shellcheck`
- the shell regression harness in `tests/test_panefleet.sh`
