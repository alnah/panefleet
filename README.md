# panefleet

Experimental repo for a tmux-first agent orchestration/workboard plugin.

## Scope

- tmux workboard UX
- agent state adapters
- Claude Code, Codex, and OpenCode integration

## Current state

Current implementation is tmux-first, with adapter ingestion available:

- popup workboard driven by `fzf`
- jump directly to a pane
- adapter-driven pane state via `bin/panefleet state-set`
- automatic pane states: `RUN`, `WAIT`, `DONE`, `ERROR`, `IDLE`
- aging states: recent `DONE` expires into `IDLE`, then `STALE`
- conservative fallback when no adapter state exists

Included integration surfaces:

- Compiled adapter bridge: [panefleet-agent-bridge](/Users/alexis/workspace/panefleet/cmd/panefleet-agent-bridge/main.go)
- Claude Code hook wrapper: [claude-code-hook](/Users/alexis/workspace/panefleet/scripts/claude-code-hook)
- Codex app-server wrapper: [codex-app-server-bridge](/Users/alexis/workspace/panefleet/scripts/codex-app-server-bridge)
- OpenCode event wrapper: [opencode-event-bridge](/Users/alexis/workspace/panefleet/scripts/opencode-event-bridge)
- OpenCode plugin shim: [opencode-panefleet.ts](/Users/alexis/workspace/panefleet/scripts/opencode-panefleet.ts)

## Requirements

Core runtime:

- `tmux` with `display-popup`
- `fzf` with `--header-lines-border`
- `ripgrep`
- `bash`

Source build:

- `go`

Adapter runtime:

- no Python runtime dependency
- OpenCode plugin integration runs inside the OpenCode plugin host

Check your runtime with:

```bash
bin/panefleet preflight
```

Run the current non-regression suite with:

```bash
./scripts/test.sh
```

## Install

### Local development

On macOS with Homebrew:

```bash
./scripts/install-deps-homebrew.sh
```

Build the compiled adapter bridge:

```bash
./scripts/build-agent-bridge.sh
```

Link the repo into the standard tmux plugin path:

```bash
mkdir -p ~/.tmux/plugins
ln -sfn "$PWD" ~/.tmux/plugins/panefleet
```

Then source the plugin in a running tmux session:

```bash
~/.tmux/plugins/panefleet/scripts/dev-source.sh
```

Default binding:

- `prefix + P` opens the panefleet workboard
- `prefix + T` opens the theme picker

### TPM-style plugin path

The repo exposes a standard `panefleet.tmux` entrypoint at the repo root.

## CLI

The main binary is:

```bash
bin/panefleet
```

Useful commands:

```bash
bin/panefleet preflight
bin/panefleet list
bin/panefleet board
bin/panefleet popup
bin/panefleet preview %1
bin/panefleet state-set --pane %1 --status RUN --tool codex --source test
bin/panefleet state-show --pane %1
bin/panefleet state-clear --pane %1
bin/panefleet-agent-bridge claude-hook
bin/panefleet-agent-bridge codex-app-server --pane %1
bin/panefleet-agent-bridge opencode-event --pane %1
bin/panefleet themes
bin/panefleet theme-apply dracula
./scripts/test.sh
```

## Themes

Panefleet ships with 11 built-in themes:

- `panefleet-dark`
- `panefleet-light`
- `dracula`
- `catppuccin-mocha`
- `tokyo-night`
- `gruvbox-dark`
- `nord`
- `solarized-dark`
- `rose-pine`
- `monokai`
- `github-dark`

Default:

- `@panefleet-theme = panefleet-dark`

Color portability:

- automatic fallback from truecolor to 256 colors, then ANSI
- optional override with `PANEFLEET_COLOR_MODE=truecolor|256|ansi`

Theme surfaces:

- popup background and border
- `fzf` prompt, header, separator, preview, selection, and borders
- board status colors
- preview body block rendering
- diff colors in preview blocks

You can switch themes with:

```bash
tmux set-option -g @panefleet-theme panefleet-light
```

Or interactively with:

- `prefix + T`

## Status model

- `RUN`: active work in progress
- `WAIT`: blocked or intentionally waiting
- `DONE`: finished enough to revisit later
- `IDLE`: shell is alive but no strong sign of active work
- `STALE`: left open without interaction beyond the configured threshold
- `ERROR`: pane process exited non-zero

Manual status overrides take precedence over inferred status.
Manual overrides exist only as a temporary fallback. The intended end state is fully automatic state detection.

Current fallback behavior when no adapter state exists:

- Codex:
  - recent approval prompt -> `WAIT`
  - recent strong error text -> `ERROR`
  - recent working footer/activity hint -> `RUN`
  - recent input prompt -> `DONE`
  - otherwise -> `IDLE`
- Claude Code:
  - recent approval or chooser prompt -> `WAIT`
  - recent strong error text -> `ERROR`
  - recent ready prompt / assistant-ready copy -> `DONE`
  - otherwise -> `RUN`
- OpenCode:
  - recent approval/permission prompt -> `WAIT`
  - recent strong error text -> `ERROR`
  - recent build/footer chrome -> `RUN`
  - recent ready/help prompt -> `DONE`
  - otherwise -> `IDLE`
- shell panes -> `IDLE`
- non-agent live processes -> `RUN`
- dead pane + zero exit -> `DONE`
- dead pane + non-zero exit -> `ERROR`

Limitation:

- All provider fallbacks are heuristic and based on recent pane text.
- Adapter state still wins whenever a fresh provider event exists.
- `DONE` and `ERROR` inferred from pane text are best effort only and can be wrong if the provider UI changes or if matching text appears in normal output.

State aging:

- `DONE` is only a recent-completion state
- after `@panefleet-done-recent-minutes`, `DONE` decays to `IDLE`
- after `@panefleet-stale-minutes` without interaction, `IDLE` decays to `STALE`

Default timing:

- `@panefleet-done-recent-minutes = 10`
- `@panefleet-stale-minutes = 45`

## Adapter roadmap

The preferred model is agent-aware adapters. Heuristic pane-text fallback exists as a backup for all providers.

### Claude Code

Included bridge:

- [claude-code-hook](/Users/alexis/workspace/panefleet/scripts/claude-code-hook)

Usage shape:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "~/.tmux/plugins/panefleet/scripts/claude-code-hook"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "~/.tmux/plugins/panefleet/scripts/claude-code-hook"
          }
        ]
      }
    ],
    "Notification": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "~/.tmux/plugins/panefleet/scripts/claude-code-hook"
          }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "~/.tmux/plugins/panefleet/scripts/claude-code-hook"
          }
        ]
      }
    ]
  }
}
```

Important:

- Claude loads hooks from the settings schema above; each event entry uses `matcher + hooks`, with `""` meaning match all.
- Existing Claude sessions may need a restart before new hooks apply.

Current bridge mapping:

- `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart` -> `RUN`
- `Notification` -> `WAIT`
- `Stop`, `SubagentStop`, `SessionEnd`, `PreCompact` -> `DONE`
- payloads containing explicit failure markers still promote `ERROR`

Smoke test:

```bash
rm -f ~/.local/state/panefleet/events/claude-hook.jsonl
claude -p 'Run pwd, then answer with the directory only.'
tail -n +1 ~/.local/state/panefleet/events/claude-hook.jsonl
```

Expected events:

- `PreToolUse`
- `PostToolUse`
- `Stop`

### Codex

Preferred path:

- [codex-app-server-bridge](/Users/alexis/workspace/panefleet/scripts/codex-app-server-bridge)
- map `thread/status/changed` notifications into pane-local state

Usage shape:

```bash
<codex app-server notification stream> | ~/.tmux/plugins/panefleet/scripts/codex-app-server-bridge --pane "$TMUX_PANE"
```

State mapping target:

- thread active -> `RUN`
- waiting on approval or blocked interaction -> `WAIT`
- completed turn with no active follow-up -> `DONE`
- app-server/system failure -> `ERROR`
- remove manual marking once app-server coverage is sufficient

### OpenCode

Included bridge:

- [opencode-event-bridge](/Users/alexis/workspace/panefleet/scripts/opencode-event-bridge)
- [opencode-panefleet.ts](/Users/alexis/workspace/panefleet/scripts/opencode-panefleet.ts)

Expected plugin wiring:

- plugin subscribes to session/tool lifecycle events
- plugin shim forwards event JSON into `opencode-event-bridge --pane "$TMUX_PANE"`
- `panefleet` stores authoritative pane state from those events

Current bridge mapping:

- `session.idle` -> `DONE`
- `session.error` -> `ERROR`
- `session.status` with `busy|running|active` -> `RUN`
- `tool.execute.before*` -> `RUN`
- `tool.execute.after*` -> `RUN`, or `ERROR` on explicit failed/error status
- `permission.asked` -> `WAIT`
- `permission.replied` -> `RUN` on explicit approval, `ERROR` on explicit denial

## Error detection

`ERROR` should not be guessed from generic inactivity. It should be promoted only from a strong signal.

Strong signals:

- pane process exited non-zero
- adapter event reports failure, abort, or system error
- tool lifecycle indicates command/tool execution failure

Weak signals that should not alone become `ERROR`:

- no recent output
- user stopped looking at the pane
- shell prompt is visible

Current behavior:

- dead pane + non-zero exit status -> `ERROR`
- dead pane + zero exit status -> `DONE`

Future adapter behavior:

- Claude Code hook failures can explicitly mark `ERROR`
- Codex app-server `systemError`-style states should map to `ERROR`
- OpenCode tool/session failure events should map to `ERROR`

## Target end state

The final product should not depend on manual `RUN`, `WAIT`, `DONE`, or `ERROR` shortcuts for normal operation.

Desired model:

- adapters emit authoritative lifecycle state
- tmux only renders and navigates
- manual overrides remain optional emergency controls, not primary workflow
- conservative liveness fallback remains only when no adapter state is present
