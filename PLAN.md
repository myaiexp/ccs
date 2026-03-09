# CCS as Full Claude Wrapper/Multiplexer

## Goal
Make ccs the primary entry point for all claude interactions. It manages claude
process lifecycles, captures their output, and provides real-time visibility into
all running instances from the main TUI.

## Current State
- ccs discovers sessions from `~/.claude/projects/` JSONL files
- Launches `claude` via tmux windows (hub mode) or inline `tea.Exec`
- Activity monitoring: watches JSONL files, shows parsed tool_use/text entries
- Tracker seeds from `/proc` to detect externally-launched claude processes

## Architecture Changes

### Phase 1: Always-tmux + capture infrastructure

**1. Remove inline launch mode entirely**
- Delete `LaunchResume()`, `LaunchNew()`, `ExecFinishedMsg` handling
- `handleEnter()` always delegates to `handleHubEnter()`
- Remove `m.launching` flag and the empty-view-while-launching logic
- Keep `tea.Exec` code paths removed — ccs never suspends itself

**2. Add tmux pane capture via `capture-pane`**
- New package `internal/capture/capture.go`
- `CapturePane(windowID string, lines int) (string, error)` — runs
  `tmux capture-pane -t <windowID> -p -S -<lines>` to grab the last N lines
  of a tmux window's visible output
- This gives us raw terminal output of any claude instance without PTY plumbing
- Polled on a timer (every 1-2s for the selected session)

**3. New message types**
- `PaneCaptureMsg { SessionID string; Content string }` — periodic pane snapshot
- New tea.Cmd `paneCaptureCmd(windowID, sessionID)` that calls CapturePane

### Phase 2: Split-pane TUI with follow mode

**4. Add "follow" view state to Model**
- New field `followID string` — session ID being followed (empty = normal list)
- When non-empty, the View() renders a split layout:
  - Top: session list (compressed, fewer rows)
  - Bottom: pane capture output for the followed session
- Toggle with `f` key on selected active session
- `Esc` exits follow mode

**5. Enhanced detail pane for active sessions**
- Left column: existing session metadata
- Right column: live pane capture (raw terminal output) instead of just
  JSONL activity entries
- Keep JSONL activity as a fallback for sessions without tmux windows
  (externally launched ones detected via /proc)

**6. Pane capture polling**
- When a session is selected or followed, start polling its tmux pane
- `PaneCaptureTickMsg` fires every 1s for the active follow target
- Capture last 20-30 lines of the tmux pane
- Store in `m.paneContent map[string]string`

### Phase 3: Process lifecycle ownership

**7. All launches go through ccs**
- New sessions: `ccs` creates tmux window, tracks PID + window ID
- Resume sessions: same, with `--resume` flag
- Track window lifecycle: detect when claude exits, update status

**8. Session status enrichment**
- Add `TmuxWindowID` to `types.Session` (merge from tracker data)
- Active sessions with tmux windows show live pane content
- Active sessions without tmux windows (external) show JSONL activity only
- Exited sessions show final JSONL state

### Phase 4: Polish

**9. Keybindings for wrapper workflow**
- `Enter` on active session → switch to its tmux window (existing)
- `Enter` on inactive session → launch `claude --resume` in new window
- `f` on active session → toggle follow mode (split pane)
- `n` on project → launch new `claude` in project dir
- `Ctrl+z` / `z` → zoom follow pane to full screen

**10. Status bar improvements**
- Show count of running instances: "3 active"
- Per-session: show last tool used, elapsed time, token count

## Files to Change

| File | Change |
|------|--------|
| `internal/capture/capture.go` | **NEW** - tmux pane capture |
| `internal/tui/model.go` | Add follow mode, pane capture state, split rendering |
| `internal/tui/launch.go` | Remove inline launch, keep only tmux launch |
| `internal/tui/styles.go` | Add styles for follow pane |
| `internal/tui/keys.go` | Add `f` for follow, `z` for zoom |
| `internal/tmux/tmux.go` | Add `CapturePaneContent()` |
| `internal/types/types.go` | Add `TmuxWindowID` to Session |
| `main.go` | Remove non-tmux code path (already tmux-only) |

## Non-goals (for now)
- Custom PTY multiplexing (tmux already does this well)
- Replacing tmux entirely with a built-in terminal emulator
- Intercepting/modifying claude's I/O stream
