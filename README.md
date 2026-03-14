# panefleet

Experimental repo for a tmux-first agent orchestration/workboard plugin.

## Scope

- tmux workboard UX
- agent state adapters
- Claude Code, Codex, and OpenCode integration

## MVP

Current MVP is tmux-only:

- popup workboard driven by `fzf`
- jump directly to a pane
- manual pane states: `RUN`, `WAIT`, `DONE`
- automatic fallback states: `RUN`, `IDLE`, `DONE`, `ERROR`

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
bin/panefleet mark-current RUN
bin/panefleet mark-current WAIT
bin/panefleet mark-current DONE
bin/panefleet mark-current CLEAR
```

## Status Model

- `RUN`: active work in progress
- `WAIT`: blocked or intentionally waiting
- `DONE`: finished enough to revisit later
- `IDLE`: shell is alive but no strong sign of active work
- `ERROR`: pane process exited non-zero

Manual status overrides take precedence over inferred status.

## Notes

- Early planning docs live in `docs/`, but `docs/nocommit-*` files are intentionally ignored.
- The current planning spec starts in `docs/nocommit-plugin-spec.md`.
