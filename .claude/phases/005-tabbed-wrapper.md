# Tabbed Session Wrapper — Needs Debugging

> Implemented tabbed session wrapper architecture but it has critical bugs. 2026-03-21.

## What Was Built

8 commits implementing: tmux primitives (15 functions), IPC package (unix socket server/client, `ccs launch`/`ccs notify-exit` CLI), tab manager (window lifecycle, adoption, status line rendering, on-done callbacks), startup lifecycle (keybinding capture/restore, signal handling, cleanup), TUI refactor (removed follow mode, added attention badges, wired tab manager).

Design docs: `.claude/plans/2026-03-21-tabbed-session-wrapper-design.md` and `.claude/plans/2026-03-21-tabbed-session-wrapper-plan.md`

## Critical Bugs (not debugged yet)

All code was implemented and tests pass, but live testing reveals:

1. **Still very laggy** — j/k navigation takes seconds. Caching `ExtractConversationText` helped ~30% but major lag remains. Root cause NOT yet identified. Likely candidates:
   - `handleRefresh()` runs synchronously on 3s tick: calls `session.LoadSessions()` (re-parses JSONL), `project.ScanProjectDirs()` (walks filesystem), `computeStateStatuses()`, `eagerLoadActivities()`
   - Multiple tmux subprocess spawns per tick (pane capture, status line)
   - Possible compounding: 1s status line tick + 1s pane capture tick + 3s refresh tick overlapping

2. **tmux keybindings don't work** — `§ space`, `§ 1`, `§ 2` not functional. Possible causes:
   - `if-shell` command format may be wrong
   - `@ccs-managed` window option may not be set correctly
   - Keybinding installation may silently fail
   - Need to test: `tmux show -wv @ccs-managed` manually, `tmux list-keys -T prefix` to verify bindings

3. **No second status line visible** — tab bar not appearing. Possible causes:
   - `tmux set-option -s status 2` may not work as expected on this tmux config
   - `SetStatusFormat()` may be setting wrong option name
   - Status line format strings may have syntax errors
   - User's tmux config may override session-scoped settings

4. **No visible change from before** — dashboard looks identical to pre-refactor. Follow mode was removed, but active rows should now have colored attention badges instead of expanded status lines. Either the attention data isn't flowing or the rendering isn't triggering.

## Debugging Strategy for Next Session

1. **Start with tmux commands manually** — verify each tmux operation works in isolation:
   ```bash
   tmux show -wv @ccs-managed          # is it set?
   tmux set-option -s status 2          # does 2-line status work?
   tmux set-option -s status-format[0] "test line 1"
   tmux list-keys -T prefix | grep -E "Space|\" 1 |\" 2 "
   ```

2. **Profile the lag** — add timing logs to `handleRefresh()`, `StatusLineTickMsg`, `PaneCaptureTickMsg` to see which is slow

3. **Check keybind installation** — add error logging to `InstallCCSBindings`, verify `CaptureBindings` returns sensible values

4. **Verify attention state flow** — check that `DeriveStatus` results reach `UpdateAttention` on the tab manager, and that attention badges render in views.go

## Files Changed This Session

- `internal/tmux/tmux.go` — 15 new functions
- `internal/tmux/keybind.go` — NEW: keybinding lifecycle
- `internal/ipc/` — NEW: protocol.go, server.go, client.go
- `internal/tabmgr/` — NEW: tabmgr.go, adopt.go, statusline.go
- `internal/tui/model.go` — tabmgr integration, removed follow mode, added ticks
- `internal/tui/views.go` — simplified active rows, attention badges, removed follow view
- `internal/tui/styles.go` — attention badge colors
- `main.go` — full startup lifecycle refactor
- `CLAUDE.md` — updated for new architecture
