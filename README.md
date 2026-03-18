# ccs — Claude Code Sessions

A TUI mission control for [Claude Code](https://docs.anthropic.com/en/docs/claude-code) sessions. Three-section layout with lifecycle tracking, auto-naming, live pane capture, and activity monitoring — all from a persistent tmux interface.

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)

## Features

- **Mission control layout** — three sections: active sessions (expanded with live status), open sessions (scrollable with detail pane), and a footer with done count
- **Session lifecycle** — four states: active (PID alive), open (persisted), done (user-marked), untracked (legacy). Active sessions auto-promote to open.
- **Auto-naming** — shells out to `claude --print --model haiku` with pane capture or JSONL content. Triggers on promotion and going inactive.
- **Live pane capture** — polls tmux pane output for active sessions, displays attention state (waiting, permission, thinking, error)
- **Follow mode** — `f` enters a split view with live terminal output from the followed session
- **Activity monitoring** — watches active session JSONL files via inotify, shows latest tool use and text output in real time
- **Unified search** — `/` searches all sessions (any lifecycle state) + project directories. Results show state badges.
- **Sorting** — cycle through time/context%/size/name, toggle ascending/descending
- **Session management** — complete, reopen, rename, delete sessions
- **Preferences** — toggle relative numbers, cycle activity line count and auto-name lines
- **Vim-style navigation** — j/k, gg/G, number shortcuts across all sections

## Requirements

- **tmux** — ccs auto-bootstraps into a tmux session on launch; all sessions open as new tmux windows
- **Go 1.25+** — for building from source

## Install

Build from source:

```bash
git clone https://github.com/myaiexp/ccs.git
cd ccs
go build -o ~/.local/bin/ccs .
```

## Usage

```bash
ccs
```

On first launch, ccs creates a tmux session and runs inside it. All Claude sessions are opened as separate tmux windows — ccs stays visible in its own window.

Press `?` for keybindings.

## Key Bindings

| Key            | Action                                  |
| -------------- | --------------------------------------- |
| `1-9`          | Resume/switch to session by number      |
| `enter`        | Switch to tmux window or resume session |
| `f`            | Follow active session (split pane view) |
| `c`            | Mark session as done (complete)         |
| `o`            | Reopen a done session                   |
| `R`            | Rename session (manual name)            |
| `N`            | Auto-name session via Haiku             |
| `/`            | Toggle fuzzy search                     |
| `j/k` `↑/↓`   | Navigate up/down                        |
| `gg` / `G`     | Jump to top / bottom                    |
| `s`            | Cycle sort: time → ctx% → size → name  |
| `r`            | Reverse sort direction                  |
| `d`            | Delete session (with confirmation)      |
| `h`            | Toggle showing done/untracked sessions  |
| `p`            | Preferences                             |
| `?`            | Help overlay                            |
| `q` / `ctrl+c` | Quit                                    |

## Configuration

`~/.config/ccs/config.toml`:

```toml
hidden_sessions = ["session-uuid-1"]
claude_flags = ["--dangerously-skip-permissions"]
relative_numbers = false
tmux_session_name = "ccs"
activity_lines = 5
auto_name_lines = 20
```

## Architecture

```
main.go                       Entry point, tmux bootstrap, session discovery → TUI
internal/
  types/types.go              Session, Config, StateStatus, ActiveSource, sort types
  config/config.go            TOML config load/save with defaults
  session/
    parse.go                  JSONL streaming parser, session discovery, project dir decoding
    cache.go                  File metadata cache (skip re-parsing unchanged files)
    tracker.go                PID-based session tracking with tmux window ID support
  state/state.go              Session lifecycle state (open/done, auto/manual naming)
  naming/naming.go            Haiku invocation for auto-naming
  project/project.go          ScanProjectDirs for ~/Projects/ listing
  tmux/tmux.go                tmux CLI: bootstrap, new-window, select-window, capture-pane
  capture/capture.go          PaneSnapshot, DeriveStatus (attention state detection)
  activity/activity.go        JSONL activity extraction (tool_use, text entries)
  watcher/watcher.go          fsnotify file watcher with per-file debounce
  tui/
    model.go                  Bubbletea model: three-section layout, state integration
    views.go                  Render functions: active rows, open rows, detail pane, search
    styles.go                 Lipgloss style definitions
    launch.go                 Tmux window launch/switch commands
```

## Known Limitations

- **Open session detection** — PID tracking works best for sessions launched from ccs. External sessions are detected via `/proc` scanning and matched by creation-time proximity, which may be ambiguous when multiple sessions in the same project start around the same time.
- **Pane capture persistence** — captured terminal output is in-memory only; it doesn't persist across ccs restarts. If a tmux window closes before capture, content is lost.
- **Linux-only /proc scanning** — the `/proc`-based external session detection only works on Linux.

## License

MIT
