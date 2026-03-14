# panefleet

Experimental repo for a tmux-first agent orchestration/workboard plugin.

## Scope

- tmux workboard UX
- agent state adapters
- Claude Code, Codex, and OpenCode integration

## Current state

Current implementation is tmux-only:

- popup workboard driven by `fzf`
- jump directly to a pane
- automatic pane states: `RUN`, `WAIT`, `DONE`, `ERROR`, `IDLE`
- agent-aware heuristics for `Codex`, `Claude Code`, and `OpenCode`

This first version does not yet integrate with Claude Code, Codex, or OpenCode events.

## Requirements

- `tmux`
- `fzf`
- `bash`

## Install

### Local development

Source the plugin file in a running tmux session:

```bash
~/workspace/panefleet/scripts/dev-source.sh
```

Default binding:

- `prefix + P` opens the panefleet workboard

### TPM-style plugin path

The repo exposes a standard `panefleet.tmux` entrypoint at the repo root.

## CLI

The main binary is:

```bash
bin/panefleet
```

Useful commands:

```bash
bin/panefleet list
bin/panefleet board
bin/panefleet preview %1
```

## Status model

- `RUN`: active work in progress
- `WAIT`: blocked or intentionally waiting
- `DONE`: finished enough to revisit later
- `IDLE`: shell is alive but no strong sign of active work
- `ERROR`: pane process exited non-zero

Manual status overrides take precedence over inferred status.
Manual overrides exist only as a temporary fallback. The intended end state is fully automatic state detection.

Current automatic behavior:

- Codex:
  - visible input prompt -> `DONE`
  - approval/confirm prompt -> `WAIT`
  - otherwise -> `RUN`
- Claude Code:
  - confirm/choice prompt -> `WAIT`
  - otherwise -> `RUN`
- OpenCode:
  - visible composer/home prompt -> `DONE`
  - approval/permission prompt -> `WAIT`
  - otherwise -> `RUN`

## Adapter roadmap

The current implementation is tmux-only. The next step is agent-aware adapters.

### Claude Code

Planned integration:

- use official Claude Code hooks
- write panefleet state on hook events
- map active tool phases to `RUN`
- map approval or user-blocked states to `WAIT`
- map stop/completion events to `DONE`
- remove the need for manual tmux status marking in normal usage

Expected implementation shape:

- small shell adapter scripts
- local state updates keyed by tmux pane id

### Codex

Planned integration:

- preferred path: Codex app-server events
- fallback path: Codex `notify` plus process liveness

State mapping target:

- thread active -> `RUN`
- waiting on approval or blocked interaction -> `WAIT`
- completed turn with no active follow-up -> `DONE`
- app-server/system failure -> `ERROR`
- remove manual marking once app-server coverage is sufficient

### OpenCode

Planned integration:

- OpenCode plugin subscribing to session and tool lifecycle events
- map active execution to `RUN`
- map idle/waiting session states to `WAIT` or `DONE` depending on event semantics
- remove manual marking once plugin events are wired in

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

## Notes

- Early planning docs live in `docs/`, but `docs/nocommit-*` files are intentionally ignored.
- The current planning spec starts in `docs/nocommit-plugin-spec.md`.
