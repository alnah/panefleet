# PANEFLEET MEMORY

## Current sequence

`work -> correct -> safe -> clean -> observable -> operable -> fast -> scalable`

## Current phase

`operable`

## Active slice

- Problem: Panefleet already had useful operator tools (`doctor`, shell
  healthcheck, backup/restore scripts, bridge event logs), but they did not yet
  form a clean operability contract:
  - no backend health endpoint with explicit `liveness` / `readiness`
  - shell healthcheck and Go runtime health were not aligned
  - runbook steps existed in pieces but not in one operator document
- Delivery mode:
  - add the narrowest structured health surface first
  - keep checks non-destructive
  - reuse existing backup/restore/resync tools instead of inventing a new
    incident framework

## Recent commits

- `15f43be` `docs: add board tui migration plan`
- `fe24655` `feat: ship go-backed panefleet board`
- `015d9dc` `fix: keep go board opt-in until parity`
- `b6b773d` `feat: harden bubble tea board parity`

## In-flight changes

- A backend health contract now exists in the Go CLI:
  - `scripts/panefleet-go health --check liveness`
  - `scripts/panefleet-go health --check readiness`
  - `liveness` checks config + local DB path viability without mutating schema
  - `readiness` adds tmux binary lookup, tmux session presence, and tmux
    snapshot ability
- Search-bar editing in the Go board now follows the requested shortcut split:
  - `ctrl+backspace` clears the whole query
  - `alt+backspace` deletes one word
  - implementation stays localized in `internal/tui` and treats `ctrl+h` as the
    terminal-level form of `ctrl+backspace`
- `scripts/ops-healthcheck.sh` now composes:
  - shell `preflight`
  - Go backend `health --check liveness`
  - Go backend `health --check readiness` when inside tmux
  - `doctor --install`
  - `doctor`
- An operator runbook now exists in `docs/operable-runbook-2026-03-27.md`:
  - health
  - diagnostics
  - backup / restore
  - controlled tmux resync via `sync-tmux --source ops:backfill`
  - bridge event log usage during incidents
- A clean audit now exists in `docs/clean-audit-2026-03-27.md`.
- `internal/board` now models row metrics as a single board concern:
  - `Service` uses a `tool -> resolver` map instead of three separate resolver
    fields
  - shared `rowMetrics` naming replaces provider-specific generic naming
  - dead `displayStatus` is gone
- `internal/tui` is now aligned with the actual board rendering contract:
  - alternate-row styling leftovers are removed
  - `renderTableRow` no longer carries a dead alternate-row argument
  - `copyStatusPtr` now matches the same pointer-copy vocabulary as
    `internal/board`
- `cmd/panefleet-agent-bridge` now uses canonical domain statuses:
  - mapping functions return `internal/state.Status`
  - transport helpers accept `state.Status` instead of bridge-local strings
  - logging still serializes strings only at the outer edge
- `cmd/panefleet` now asks one DB-path question instead of two:
  - `shouldManageDBPath` replaces the split between path preparation and
    permission hardening helpers
- `lib/` shell helpers are slightly tighter:
  - `go_board_command_path` better describes the Go board launcher role
  - duplicate double-quoted escaping helpers were collapsed into one
    `double_quote_literal_escape`
  - bridge-wrapper variable names now reflect CLI vs ingest responsibilities
- The board renderer now has:
  - a stripped-down chrome after screenshot review removed decorative title, pills, counters, and noisy top summaries
  - explicit but minimal `BOARD` and `PREVIEW` section labels
  - aligned headers, a stronger selected row marker, and less ornamental table styling
  - a compact preview summary with fewer labels and a quieter metadata treatment
  - a main content area that now prefers a horizontal split:
    - the dashboard stays on the left
    - the session preview stays on the right
    - the split now targets `3/5 + 2/5` when the terminal is wide enough
    - narrow terminals still fall back to the older vertical stack to avoid unreadable panes
    - each rendered line in the horizontal split is now padded to the full viewport width so stale tmux content does not bleed through when one pane is shorter than the other
    - preview summary, metadata, and body lines now wrap inside the right pane instead of being hard-truncated by column width
  - a top search prompt in the form `Panefleet > ...`
  - a simplified interaction model:
    - printable keys feed search directly
    - `Enter` jumps to the selected pane and now exits the board immediately on success so tmux focus becomes visible
    - `Ctrl+S` toggles stale on the selected pane
    - arrows remain available for moving inside filtered results
    - `Esc` now quits again as advertised
    - `alt+backspace` now clears the full search line; plain `Backspace` still deletes one character
  - stale toggles now update the selected row optimistically before the async refresh completes
- The Codex ingest launcher now:
  - changes directory to the panefleet repo root before running `go run ./cmd/panefleet`
  - is protected by a shell contract test that runs the script from outside the repo and asserts both cwd and argv
- Provider bridge wrappers now default `PANEFLEET_INGEST_BIN` back to `scripts/panefleet-go`:
  - `bin/panefleet` does not expose `ingest`, so the wrapper had silently broken live provider metrics with `unknown command: ingest`
  - the wrapper still runs from the repo root, so provider hooks remain location-independent
- The OpenCode plugin template is now best-effort:
  - bridge stderr is ignored instead of inherited into the pane UI
  - spawn and stdin write failures are swallowed
  - the plugin no longer waits for bridge exit before returning control
- The Go board now also falls back to live Codex metrics when tmux options are empty:
  - it reads `pane_pid` from tmux
  - finds the descendant `codex` process under that pane shell
  - extracts the Codex thread id from the open `rollout-...jsonl` file
  - reads `tokens_used` and model context metadata from `~/.codex/state_*.sqlite` and `models_cache.json`
  - fills `TOKENS` / `CTX%` in the board rows without waiting for tmux option ingestion
- The Go bridge now enriches provider metrics directly:
  - Claude `Stop` hooks read the transcript JSONL usage block and set `TOKENS` / `CTX%`
  - OpenCode event hooks read `tokens.total` and `modelID` from live plugin payloads and set metrics even when the event does not map to a lifecycle state
- The Go board now also falls back to provider-local stores when tmux options are empty:
  - Claude rows resolve the latest transcript under `~/.claude/projects/<cwd-slug>/` and derive usage from the latest assistant message
  - OpenCode rows resolve the latest matching session from `~/.local/share/opencode/opencode.db` and read assistant/session token usage directly from sqlite
  - context percentages are derived from Codex model metadata when available, with a Claude fallback context window of `200000`
- The board jump flow is now:
  - protected by a TUI test that requires `Enter` to return `tea.Quit` after a successful jump
  - intentionally non-refreshing on success because the program exits instead of repainting the board
- Tool classification now also uses `window_name`:
  - Claude panes whose `pane_current_command` is only a version string like `2.1.85` are still rendered as `claude`
  - this behavior is locked by a board service test
- Claude wait heuristics in the Go board are now narrowed to real chooser prompts:
  - generic `Press` text no longer forces `WAIT`
  - chooser flows that include `Press ... navigate` still resolve to `WAIT`
  - a Claude prompt ending in bare `❯` now remains `DONE` even when surrounding prose mentions permissions
- The Go board color theme mapping is now quieter and more semantic:
  - column headers and section labels use muted structural color instead of loud accent color
  - selected rows rely on an accent marker plus a restrained surface tint instead of a heavy high-contrast wash
  - `TOKENS` and `CTX%` now use dedicated metric colors rather than recycling lifecycle status colors
  - preview headings now use accent color while list/body/meta text stay closer to the base foreground hierarchy
- The Go board row selection now renders as a true horizontal bar:
  - the selected row background is applied segment by segment across the full row width, including separators and trailing space
  - alternating row background stripes have been removed from session rows after screenshot review showed they added noise
- Tests still pass after the clean pass and prior visual/provider refactors:
  - `go test ./internal/tui`
  - `go test ./internal/...`
  - `go test ./cmd/...`
  - `go test ./...`
  - `bash tests/test_panefleet.sh`

## Current product state

- `board` and `popup` now route to the Bubble Tea board again.
- `board-shell` and `popup-shell` remain explicit fallbacks.
- The Go board now has theme-aware rendering and stronger tmux heuristics.
- `internal/board` now resolves status with explicit live precedence instead of letting stale store or stale adapter values freeze the board.
- Deterministic tests protect rows and preview for:
  - live tmux over stored projection
  - live capture over stale stored state
  - codex live wait over fresh adapter done
  - opencode live done over fresh adapter run
- Deterministic TUI tests now also lock the degraded-refresh contract so the board keeps its last good rows and preview on fetch errors.
- Deterministic TUI tests also lock timeout propagation for both rows and preview fetch paths.
- The current UI direction is now much more austere: less “terminal porn”, more operator scan speed.
- The current board interaction is now search-first rather than shortcut-first.
- The current board interaction contract is now again consistent with the displayed controls.
- The current OpenCode integration contract now explicitly avoids pane pollution and synchronous bridge blocking.
- The current OpenCode integration contract now also surfaces token usage from provider payloads and sqlite fallback.
- The current Claude integration contract now distinguishes real chooser UIs from plain permission-related prose in the capture.
- The current Claude integration contract now also surfaces transcript-derived usage in the board.
- The current board visual direction now separates structure, metrics, and lifecycle semantics more cleanly.
- The preview pane readability pass is now in a cleaner V1 state:
  - the `PREVIEW` section bar now carries compact context (`status · tool · target · repo/window`) instead of spending separate body lines on summary chrome
  - the preview body now starts immediately below the section bar
  - decorative divider lines from captured content are dropped and repeated blank lines are collapsed
  - prose/list/quote/warning/error lines no longer inherit the code-style gutter; the gutter stays for shell and diff-like content
  - new TUI tests lock the compact section-bar metadata contract and the body-first preview rendering contract
- The preview chrome was deliberately kept austere after review:
  - wrap and scroll controls were removed from the visible TUI help line
  - the preview section bar no longer advertises wrap state
  - the board keeps the simpler jump/stale/quit control surface

## Next verification gate

- Validate the new operability contract end-to-end:
  - `scripts/panefleet-go health --check liveness`
  - `scripts/panefleet-go health --check readiness`
  - `scripts/ops-healthcheck.sh`
- Then continue the next `operable` concerns:
  - decide whether provider event-log replay needs a first-class tool or
    whether `sync-tmux --source ops:backfill` stays the preferred backfill path
  - decide whether `doctor` should also gain a stable machine-readable mode
  - add a small config reference if env var sprawl starts to become an incident
    source
