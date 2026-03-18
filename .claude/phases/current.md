# Mission Control Rework — Just Landed

> Major rework from session browser → mission control dashboard. Implemented 2026-03-18. Likely needs debugging and polish.

## What Changed

### New Architecture
- **Session lifecycle**: Active (PID alive) → Open (persisted) → Done (user-marked). Untracked = legacy sessions not in state.json.
- **Three-section layout**: Active section (expanded, live status via `DeriveStatus`), Open section (scrollable with detail pane), Done/Untracked (toggled with `h`).
- **Auto-naming**: Shells out to `claude --print --model haiku` for session names. Triggers on promotion (30s delay), on going inactive, and manual `N` key.
- **Search rework**: `/` searches all sessions + `~/Projects/` directories. Project grid removed entirely.

### New Packages
- `internal/state` — lifecycle persistence to `~/.cache/ccs/state.json`
- `internal/naming` — haiku invocation for auto-naming
- `capture.DeriveStatus()` — attention state detection from pane content

### Key Bindings Changed
- **Added**: `c` (complete), `o` (reopen), `R` (rename), `N` (auto-name)
- **Removed**: `tab`, `n`, `x`, `left/right` (all project grid related)

### Config Changed
- **Added**: `auto_name_lines` (default 20)
- **Removed**: `hidden_projects`, `project_name_max`

## Known Issues to Debug
- Navigation may feel different — unified j/k across active+open sections, no tab switching
- Auto-naming requires `claude` CLI in PATH and existing subscription
- `DeriveStatus` is regex-based, best-effort — false positives expected
- Search result → normal view cursor position handoff may not be smooth
- Mase reported it's "interesting" but needs hands-on debugging

## Design Spec
- `docs/plans/2026-03-18-mission-control-rework-design.md` — full spec
- `docs/plans/2026-03-18-mission-control-rework-plan.md` — implementation plan
- `.claude/ideas.md` — deferred ideas from brainstorming (multi-follow, activity feed, tmux alerts, stale nudges, CLI mode, frecency)
