# ccs — Claude Code Session Hub

Go TUI (bubbletea/lipgloss) that serves as a persistent tmux hub for Claude Code sessions. Shows recent sessions and projects, launches sessions as tmux windows, monitors live activity via inotify and tmux pane capture.

## Stack
- Go 1.25+
- bubbletea (TUI framework), lipgloss (styling), bubbles (textinput)
- sahilm/fuzzy (fuzzy search), BurntSushi/toml (config)
- fsnotify (inotify file watching for live activity)
- tmux capture-pane (live terminal output)

## Structure
```
main.go                    Entry point, tmux bootstrap, wires up data loading → TUI
internal/
  types/types.go           Session (with ActiveSource), Project, Config, SortField/SortDir
  config/config.go         Loads/saves ~/.config/ccs/config.toml, applies defaults
  session/
    parse.go               JSONL parsing, session discovery, project dir decoding, cleanTitle
    cache.go               File metadata cache (skip re-parsing unchanged JSONL files)
    tracker.go             PID-based session tracking with tmux window ID support
  project/project.go       Project discovery from session data
  tmux/tmux.go             tmux CLI wrapper: Bootstrap, NewWindow, SelectWindow, WindowExists, CapturePaneContent
  capture/capture.go       PaneSnapshot struct, CapturePane function (wraps tmux capture-pane)
  activity/activity.go     JSONL activity extraction: ExtractFromLine, TailFile, FormatEntry
  watcher/watcher.go       fsnotify-based file watcher with debounce, sends ActivityUpdates
  tui/
    model.go               Bubbletea Model, Init, Update, key handling, filtering, grid layout
    views.go               All View/render functions, text formatting helpers
    styles.go              Lipgloss style definitions (three-state dots, activity style, follow pane)
    keys.go                Key binding definitions
    launch.go              Tmux window launch commands (TmuxLaunchResume, TmuxLaunchNew, TmuxSwitch)
```

## Build & Install
```bash
go build -o ~/.local/bin/ccs .
go test ./... -count=1          # tests for types, config, project, session, tmux, activity, watcher, capture
```

## Key Design Decisions
- **tmux-only mode** — ccs auto-bootstraps into a tmux session on launch. All sessions open as new tmux windows; ccs never suspends (no tea.Exec). `enter` switches to existing window or creates new one.
- **Follow mode** — `f` key on an active SourceTmux session enters split view: compressed session list (top ~40%) + live tmux pane capture (bottom ~60%). Bottom-anchored scrolling shows latest output. `f` again or `Esc` exits.
- **Live pane capture** — 1s polling via PaneCaptureTickMsg. Captures ALL sessions with tmux windows. `tmux capture-pane` with HUD/spinner stripping (box-drawing separators, ✻/* spinner lines, trailing blanks). Persists after session goes inactive (rendered dimmed).
- **Three-state activity dots** — green (SourceTmux, launched from ccs), yellow (SourceProc, detected via /proc), dim (inactive)
- **Live activity monitoring** — fsnotify watches active session JSONL files. 200ms debounce per file. TailFile reads last 32KB for efficient parsing. Activity shows in session rows and detail pane.
- **Session name support** — `/session-name` renames parsed from system messages (top-level `content` field). Shown on session rows (light purple) between project name and title. Title always shows first user message.
- **Detail pane** — header: project + session name + title with right-aligned ctx%/time. Info line: dir/id + msgs + size. Pane capture below status (dimmed for inactive). Selection preserved by session ID across refreshes.
- **Periodic refresh** — 10s tick refreshes session list, diffs active set to watch/unwatch files
- **PID-based session tracking** — `~/.cache/ccs/active.json` maps session IDs to PIDs and tmux window IDs. On startup, seeds from `/proc` (--resume flag detection). On refresh, prunes dead PIDs and dead tmux windows.
- **Known gap**: sessions launched outside ccs without `--resume` won't be tracked until interacted with through ccs.
- **JSONL parsing** streams line-by-line, never loads entire file into memory
- **Title extraction**: first line of first non-meta user message, markdown stripped (cleanTitle). No hardcoded length cap — display layer truncates to fit.
- **Known gap**: pane capture for inactive sessions doesn't persist across ccs restarts (only in-memory). Capture also requires tracker to still have the tmux window ID — if the window closes before capture, content is lost.
- **Directory encoding**: regex patterns matching cc-sessions' approach (ambiguous dash encoding)
- **Global numbering**: sessions numbered 1-N in sorted order, 1-9 are keyboard shortcuts
- **Preferences popup**: `p` key opens toggleable/cycleable settings (persisted to config): relative numbers, activity lines (3/5/10/15), project name max (12/16/20/24/30)
- **lipgloss `.Width()` includes padding** — content area is `.Width() - horizontal_padding`. Must subtract padding when calculating available text width inside bordered/padded styles.

## Key Bindings
`enter/1-9` switch/resume, `f` follow (split pane), `n` new, `/` search, `tab` switch sections, `s` sort, `r` reverse, `d` delete, `x` hide session, `h` show hidden, `p` prefs, `j/k/↑↓` navigate, `gg/G` top/bottom, `←→` project grid, `?` help, `esc` exit follow/clear filter, `q` quit

## Config
`~/.config/ccs/config.toml`:
```toml
hidden_projects = ["cloned", ".claude"]
hidden_sessions = ["session-uuid-1"]
claude_flags = ["--dangerously-skip-permissions"]
relative_numbers = false     # nvim-style relative numbering (toggle with p → prefs)
tmux_session_name = "ccs"    # tmux session name for bootstrap
activity_lines = 5           # activity entries shown in detail pane (cycle: 3/5/10/15 in prefs)
project_name_max = 16        # max chars for project names in grid (cycle: 12/16/20/24/30 in prefs)
```

## Docs
- `PLAN.md` — architecture plan (all 4 phases complete) and potential future work
- `docs/plans/2026-03-07-ccs-tui-design.md` — original design doc
- `docs/plans/2026-03-07-ccs-implementation-plan.md` — implementation plan
- `.claude/phases/current.md` — phase tracking
- `.claude/ideas.md` — feature ideas (pinned sessions, save/restore active sessions)
