# CCS as Full Claude Wrapper/Multiplexer

## Goal
Make ccs the primary entry point for all claude interactions. It manages claude
process lifecycles, captures their output, and provides real-time visibility into
all running instances from the main TUI.

## Current State (Phases 1-4 complete)
- ccs discovers sessions from `~/.claude/projects/` JSONL files
- tmux-only mode: all sessions open as tmux windows, ccs never suspends
- Live pane capture: polls tmux `capture-pane` for all sessions with tmux windows
- Follow mode: split view with compressed session list + live terminal output
- Activity monitoring: fsnotify watches JSONL files, shows parsed tool_use/text entries
- Three-state tracking: SourceTmux (green), SourceProc (yellow), inactive (dim)
- PID-based tracker seeds from `/proc`, matches to tmux pane PIDs
- Session metadata caching for fast startup

## Completed Architecture

### Phase 1: Always-tmux + capture infrastructure ✓
- Removed inline launch mode entirely (no `tea.Exec`, ccs never suspends)
- `enter` always opens/switches tmux windows
- Added `internal/capture/capture.go` — tmux pane capture wrapper
- Added `PaneCaptureMsg`, `PaneCaptureTickMsg` — 1s polling for all tracked sessions

### Phase 2: Split-pane TUI with follow mode ✓
- `followID` field tracks followed session
- `f` key toggles split view: compressed list (top ~40%) + live pane (bottom ~60%)
- Bottom-anchored scrolling shows latest output
- Detail pane shows pane capture for active sessions, JSONL activity as fallback
- Dimmed pane content for inactive sessions (stale capture)

### Phase 3: Process lifecycle ownership ✓
- New sessions created via `tmux.NewWindow`, tracked with PID + window ID
- Resume sessions via `--resume` flag in new tmux window
- Dead PIDs pruned on refresh, dead tmux windows cleared
- `active.json` persists tracker state across refreshes

### Phase 4: Polish ✓
- `Enter` on active → switch to tmux window; on inactive → resume in new window
- `f` on active SourceTmux → follow mode; `Esc` exits
- `n` jumps to projects for new session launch
- Active count shown in sessions header
- HUD/spinner stripping from pane captures
- Task list collapsing in pane output

## Potential Future Work
- Persist pane captures to disk for cross-restart continuity
- Custom PTY multiplexing (not planned — tmux handles this well)
- macOS `/proc` equivalent for external session detection
- Session grouping / tagging
