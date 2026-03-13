# ccs — Claude Code Session Hub

## What It Is

A Go TUI (bubbletea/lipgloss) that serves as the entry point for Claude Code. Shows recent sessions and projects, lets you resume or start new sessions with minimal keystrokes. Launches claude via `exec`, so you run `ccs` again when you need it next.

## Layout

Single screen, two sections:

```
┌─ ccs ─────────────────────────────────────────────────┐
│ 🔍 filter...                                          │
│                                                       │
│ RECENT SESSIONS                                       │
│ ● [1] spot-price    spot-price-frontend    32% 12m    │
│ ● [2] ~             cc-sessions-md-fix     18%  5m    │
│ ○ [3] central-hub   frontend-integration   48%  1h    │
│ ○ [4] poe-proof     css-var-standard...    42%  2h    │
│                                                       │
│ PROJECTS                                              │
│   central-hub · poe-crafting · poe-proof · spot-...   │
│   tracker · mase.fi · ccs · ~                         │
│                                                       │
│ enter/1-9 resume  n new  / search  tab switch  q quit │
└───────────────────────────────────────────────────────┘
```

## Sections

### Recent Sessions

- Shows last ~20 sessions across all projects, sorted by last modified
- Each row: active indicator (●/○), number shortcut [1-9], project name, session title, context %, time ago
- Fuzzy-searchable via the filter bar
- Number shortcuts [1-9] for the top 9 sessions (press the number to instantly resume)
- Arrow/j/k to navigate, enter to resume

### Projects

- Discovered from `~/.claude/projects/` directory names (decoded)
- Shows last-active time per project
- Hidden projects configurable in `~/.config/ccs/config.toml`
- Selecting a project runs `claude` in that directory (new session)

## Active Session Detection

1. `pgrep` for claude processes
2. Read `/proc/<pid>/cwd` for each
3. Encode cwd path (`/` → `-`) to match `~/.claude/projects/` directory naming
4. Most recently modified JSONL in that project dir = the active session
5. cwd match alone marks a project as "has active session" even before JSONL exists

## Keybindings

| Key | Action |
|-----|--------|
| `1-9` | Resume session by number (instant) |
| `enter` | Resume selected session / new session in selected project |
| `n` | New session (jump to project picker) |
| `/` | Focus search/filter bar |
| `tab` | Switch focus: sessions ↔ projects |
| `d` | Delete session JSONL (with confirmation) |
| `h` | Toggle showing hidden projects |
| `?` | Help overlay |
| `q` / `ctrl+c` | Quit |
| `j/k` / arrows | Navigate |

## Session Resume Flow

1. Determine project directory from JSONL path (decode `-home-mse-Projects-foo` → `~/Projects/foo`)
2. `os.Chdir` to project directory
3. `syscall.Exec` into `claude --resume <session-id>` (replaces ccs process)

## New Session Flow

1. User selects a project from the projects section
2. `os.Chdir` to project directory
3. `syscall.Exec` into `claude` (no flags, fresh session)

## Configuration

`~/.config/ccs/config.toml`:

```toml
# Projects to hide from the projects list
hidden_projects = [".claude", "cloned"]

# Default claude flags to pass through
claude_flags = ["--dangerously-skip-permissions"]
```

## Data Source

Reads `~/.claude/projects/*/` JSONL files directly. Same data as `cc-sessions` but parsed in Go. The `cc-sessions` bash script remains as a non-interactive fallback (useful for CC agents to search/read sessions).

## Project Structure

```
~/Projects/ccs/
├── main.go           # Entry point, arg parsing
├── tui/
│   ├── model.go      # Main bubbletea model
│   ├── sessions.go   # Session list component
│   ├── projects.go   # Project list component
│   ├── search.go     # Fuzzy filter
│   └── styles.go     # Lipgloss styles
├── session/
│   ├── parse.go      # JSONL parsing
│   ├── detect.go     # Active session detection via /proc
│   └── types.go      # Session/Project types
├── config/
│   └── config.go     # TOML config loading
├── go.mod
└── go.sum
```

## Install

`go build -o ~/.local/bin/ccs`

## Future Ideas (Not in V1)

- **Persistent wrapper mode**: ccs stays running, spawns claude as child, redraws when it exits. Would make it a true "session manager" but conflicts with multi-session workflow.
- **Session preview pane**: press `p` to see first few exchanges without entering the session.
- **Multi-session management**: track all running claude sessions, switch between them (would need terminal multiplexer integration).
- **Session notes/tags**: user-defined metadata on sessions beyond auto-generated titles.
- **Remote sessions**: show/resume sessions from the laptop via SSH.
