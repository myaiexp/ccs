# Hub Mode — Design Doc

## Overview

Transform ccs from a "launch and exit" tool into a persistent session hub that stays open while Claude Code sessions run in separate tmux windows. Adds live activity monitoring so you can see what each session is doing without switching to it.

## Architecture

ccs runs inside tmux as one window. Sessions open as new tmux windows in the same tmux session. Inotify watches active session JSONL files for real-time activity updates. Legacy inline mode (current behavior) remains available via `o` keybind.

## tmux Integration

### Launching Sessions

- `enter` on a session → `tmux new-window -n "proj: title" 'claude --resume <id>'` with working dir set to project dir
- `enter` on a project → `tmux new-window -n "proj: new" 'claude [flags]'` in project dir
- `o` on either → legacy inline mode (current `tea.Exec` suspend/resume behavior)
- Window name: `"project-name: session-title"` truncated to ~30 chars

### tmux Awareness

- ccs checks `$TMUX` env var to determine if it's inside tmux
- If inside tmux: creates windows in the current session
- If not inside tmux: starts tmux with ccs as the first window (`tmux new-session -s ccs 'ccs'`)
- Config option `tmux_session_name` for customization (default: `"ccs"`)

### Session Lifecycle

- Tracker records tmux window ID alongside PID when launching
- `enter` on an already-active session with a tmux window → `tmux select-window -t <window-id>` (switches to it instead of duplicating)
- `enter` on an active session without a tmux window → opens new tmux window with `--resume`
- When claude exits, tmux window closes, PID dies, tracker prunes as normal

## Live Activity Monitoring

### Data Source

- Inotify watches on JSONL files for active sessions only
- On file modification, tail last few lines to extract latest activity
- Session list discovery (new/deleted sessions, tracker prune): periodic poll every 10s
- No watching overhead for inactive sessions

### What Gets Extracted

- Tool calls: `"Read src/main.go"`, `"Edit internal/tui/model.go"`, `"Bash: go test ./..."`, `"Grep: handleRefresh"`
- Assistant text: truncated first line, e.g. `"Fixed the import issue..."`
- Filtered out: meta messages, system prompts, user messages — focus on what claude is doing

### Activity in Session List Row

Current layout:
```
● 1 project-name  Title (truncated)              85%  3m
```

New layout:
```
● 1 project-name  First message fills available space here...  Editing model.go  85%  3m
```

- First message fills all available row space (fix: currently truncated too aggressively)
- Activity status sits right-aligned, between first message and context%
- Inactive sessions: no activity shown, first message fills that space

### Visual Indicators

- `●` green — active, has a tmux window (launched from ccs)
- `●` yellow — active, running externally (found via /proc, no tmux window)
- `○` dim — inactive

## Detail Pane — Two-Column Layout

```
┌─────────────────────────────────────────────────────────────┐
│ project-name  Session title here              85%  3m ago   │
│                                                             │
│ ┌── Info ──────────────┐ ┌── Activity ────────────────────┐ │
│ │ Project /home/mse/... │ │ Edit internal/tui/model.go    │ │
│ │ Messages 42  Size 1MB │ │ Bash: go test ./...           │ │
│ │ ● active              │ │ Read config.go                │ │
│ │ ID abc-123-def        │ │ "Fixed the import issue..."   │ │
│ │                       │ │ Grep: "handleRefresh"         │ │
│ │ First message here,   │ │                               │ │
│ │ now properly wrapping │ │                               │ │
│ │ to fill the column... │ │                               │ │
│ └───────────────────────┘ └───────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

- Left column (~40%): project dir, messages, size, status, ID, first message (word-wrapped to fill)
- Right column (~60%): activity log, latest at top, auto-updates via inotify
- Height: `max(left_content, activity_lines)` — neither column clips
- Default activity lines: 5 (configurable via prefs)
- Inactive sessions: right column hidden or shows "no activity", left column fills full width (current behavior)

## Footer Hints

- Active session with tmux window selected: `enter switch` instead of `enter resume`
- Always shows: `enter switch/resume  o inline  n new  / search  tab switch  ...`

## Preferences Additions

- `activity_lines` — number of entries in detail pane activity log (default: 5)
- Activity type filtering — which tool types to show/hide in activity
- More to be added as the feature matures based on usage feedback

## Config Additions

```toml
tmux_session_name = "ccs"  # tmux session name when auto-starting
```

## Keybind Changes

| Key     | Current             | Hub mode                                    |
| ------- | ------------------- | ------------------------------------------- |
| `enter` | Resume inline       | Open/switch tmux window (hub), resume if no tmux |
| `o`     | (unused)            | Legacy inline mode (current behavior)       |
| `1-9`   | Resume inline       | Open/switch tmux window (hub)               |

## Non-Goals

- tmux configuration — user's responsibility, ccs just calls tmux commands
- Embedded terminal output in ccs (future idea, see ideas.md)
- Replacing tmux — ccs is a session manager, tmux handles terminal multiplexing
