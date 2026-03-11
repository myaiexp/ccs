# ccs — Claude Code Sessions

A TUI hub for managing [Claude Code](https://docs.anthropic.com/en/docs/claude-code) sessions. Browse, resume, follow, and launch sessions — all from a persistent tmux interface.

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)

## Features

- **Session browser** — lists all Claude Code sessions with project name, title, context %, message count, and age
- **Resume & launch** — resume any session with a keypress (1-9 shortcuts) or launch a new one in a project directory
- **Live pane capture** — polls tmux pane output for active sessions, displays in the detail pane (with HUD/spinner stripping)
- **Follow mode** — `f` enters a split view: compressed session list + full live terminal output from the followed session
- **Activity monitoring** — watches active session JSONL files via inotify, shows latest tool use and text output in real time
- **Three-state tracking** — green dot (launched from ccs, has tmux window), yellow dot (detected via /proc), dim (inactive)
- **Fuzzy search** — filter sessions and projects by name, session name, or title
- **Sorting** — cycle through time/context%/size/name, toggle ascending/descending
- **Session management** — hide, delete, show hidden sessions
- **Project grid** — columnar project layout with keyboard navigation
- **Preferences** — toggle relative numbers, cycle activity line count and project name length
- **Vim-style navigation** — j/k, gg/G, number shortcuts

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
| `n`            | Jump to projects section                |
| `/`            | Toggle fuzzy search                     |
| `tab`          | Switch between sessions and projects    |
| `j/k` `↑/↓`    | Navigate up/down                        |
| `←/→`          | Navigate project grid                   |
| `gg` / `G`     | Jump to top / bottom                    |
| `s`            | Cycle sort: time → ctx% → size → name   |
| `r`            | Reverse sort direction                  |
| `d`            | Delete session (with confirmation)      |
| `x`            | Hide/unhide session                     |
| `h`            | Toggle showing hidden items             |
| `p`            | Preferences                             |
| `?`            | Help overlay                            |
| `q` / `ctrl+c` | Quit                                    |

## Configuration

`~/.config/ccs/config.toml`:

```toml
hidden_projects = ["cloned", ".claude"]
hidden_sessions = ["session-uuid-1"]
claude_flags = ["--dangerously-skip-permissions"]
relative_numbers = false
tmux_session_name = "ccs"
activity_lines = 5
project_name_max = 16
```

## Architecture

```
main.go                       Entry point, tmux bootstrap, session discovery → TUI
internal/
  types/types.go              Session, Project, Config, sort types
  config/config.go            TOML config load/save with defaults
  session/
    parse.go                  JSONL streaming parser, session discovery, parallel workers
    cache.go                  File metadata cache (skip re-parsing unchanged files)
    tracker.go                PID-based session tracking with tmux window ID support
  project/project.go          Project discovery from session data
  tmux/tmux.go                tmux CLI: bootstrap, new-window, select-window, capture-pane
  capture/capture.go          PaneSnapshot struct, wraps tmux capture-pane
  activity/activity.go        JSONL activity extraction (tool_use, text entries)
  watcher/watcher.go          fsnotify file watcher with per-file debounce
  tui/
    model.go                  Bubbletea model, view, update, key handling
    styles.go                 Lipgloss style definitions
    keys.go                   Key binding definitions
    launch.go                 Tmux window launch/switch commands
```

## Known Limitations

- **Open session detection** — PID tracking works best for sessions launched from ccs. External sessions are detected via `/proc` scanning and matched by creation-time proximity, which may be ambiguous when multiple sessions in the same project start around the same time.
- **Pane capture persistence** — captured terminal output is in-memory only; it doesn't persist across ccs restarts. If a tmux window closes before capture, content is lost.
- **Linux-only /proc scanning** — the `/proc`-based external session detection only works on Linux.

## License

MIT
