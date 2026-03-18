# ccs — Claude Code Session Hub

Go TUI (bubbletea/lipgloss) that serves as a mission control dashboard for Claude Code sessions. Three-section layout: active sessions (expanded with live status), open sessions (scrollable with detail pane), and a footer with done count. Session lifecycle states (active/open/done), auto-naming via Haiku, and unified search replacing the project grid.

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
  types/types.go           Session (with StateStatus, ActiveSource), Config, SortField/SortDir
  config/config.go         Loads/saves ~/.config/ccs/config.toml, applies defaults
  session/
    parse.go               JSONL parsing, session discovery, project dir decoding, cleanTitle
    cache.go               File metadata cache (skip re-parsing unchanged JSONL files)
    tracker.go             PID-based session tracking with tmux window ID support
  state/state.go           Session lifecycle state + names (open/done, auto/manual naming)
  naming/naming.go         Haiku invocation for auto-naming via `claude --print --model haiku`
  project/project.go       ScanProjectDirs for ~/Projects/ directory listing
  tmux/tmux.go             tmux CLI wrapper: Bootstrap, NewWindow, SelectWindow, WindowExists, CapturePaneContent
  capture/capture.go       PaneSnapshot, CapturePane, DeriveStatus (attention state detection)
  activity/activity.go     JSONL activity extraction: ExtractFromLine, TailFile, FormatEntry
  watcher/watcher.go       fsnotify-based file watcher with debounce, sends ActivityUpdates
  tui/
    model.go               Bubbletea Model, three-section layout, state integration, naming triggers
    views.go               Render functions: active rows, open rows, detail pane, search results, footer
    styles.go              Lipgloss style definitions (badges, status, activity, follow pane)
    launch.go              Tmux window launch commands (TmuxLaunchResume, TmuxLaunchNew, TmuxSwitch)
```

## Build & Install

```bash
go build -o ~/.local/bin/ccs .
go test ./... -count=1          # tests for all packages
```

## Key Design Decisions

- **Session lifecycle** — four states: Active (PID alive, computed), Open (persisted in state.json), Done (user-marked), Untracked (legacy). Active sessions auto-promote to Open. State stored in `~/.cache/ccs/state.json`.
- **Three-section layout** — Active section (always visible, expanded rows with live status), Open section (scrollable, selected shows detail pane), Done/Untracked (toggled with `h`). Unified j/k navigation across all sections.
- **Auto-naming** — shells out to `claude --print --model haiku --no-session-persistence` with pane capture or JSONL tail content. Triggers: 30s after promotion, on session going inactive, manual `N` key. Manual names (`R`) never overwritten.
- **Display name fallback** — manual name > auto name > /session-name > first user message title
- **Attention states** — `DeriveStatus()` scans pane content bottom-up: waiting prompt, permission prompt, thinking/spinner, error, or fallback to last content line. Fast pattern matching (1s polling).
- **Search rework** — `/` searches all sessions (any lifecycle state) + project directories at `~/Projects/`. Results show state badges: `●` active, `○` open, `✓` done, `·` untracked, `▸` project dir.
- **tmux-only mode** — auto-bootstraps into tmux. All sessions open as new tmux windows.
- **Follow mode** — `f` key on active SourceTmux session enters split view with live pane capture.
- **Live pane capture** — 1s polling. Captures all sessions with tmux windows. Persists after inactive (dimmed).
- **Live activity monitoring** — fsnotify watches active JSONL files. 200ms debounce.
- **PID-based tracking** — `~/.cache/ccs/active.json` maps session IDs to PIDs and tmux window IDs.
- **JSONL parsing** streams line-by-line, never loads entire file into memory
- **Known gap**: pane capture for inactive sessions doesn't persist across ccs restarts (only in-memory).
- **lipgloss `.Width()` includes padding** — must subtract padding for text width calculations.

## Key Bindings

`enter` switch/resume, `1-9` shortcuts, `f` follow, `c` complete, `o` reopen, `R` rename, `N` auto-name, `/` search, `s` sort, `r` reverse, `d` delete, `h` toggle done/untracked, `p` prefs, `j/k/↑↓` navigate, `gg/G` top/bottom, `?` help, `esc` exit follow/clear filter, `q` quit

## Config

`~/.config/ccs/config.toml`:

```toml
hidden_sessions = ["session-uuid-1"]
claude_flags = ["--dangerously-skip-permissions"]
relative_numbers = false     # nvim-style relative numbering (toggle with p → prefs)
tmux_session_name = "ccs"    # tmux session name for bootstrap
activity_lines = 5           # activity entries shown in detail pane (cycle: 3/5/10/15 in prefs)
auto_name_lines = 20         # lines fed to haiku for naming (cycle: 10/20/30/50 in prefs)
```

## Doc Management

This project splits documentation to minimize context usage. Follow these rules:

### File layout

| File                         | Purpose                                                        | When to read                              |
| ---------------------------- | -------------------------------------------------------------- | ----------------------------------------- |
| `CLAUDE.md` (this file)      | Project identity, structure, patterns, current phase pointer   | Auto-loaded every session                 |
| `.claude/phases/current.md`  | Symlink → active phase file                                    | Read when starting phase work             |
| `.claude/phases/NNN-name.md` | Phase files (active via symlink, completed ones local-only)    | Only if you need historical context       |
| `.claude/ideas.md`           | Future feature ideas, tech debt, and enhancements              | When planning next phase or brainstorming |
| `.claude/plans/`             | Design docs and implementation plans from brainstorming        | When implementing or reviewing designs    |
| `.claude/references/`        | Domain reference material (specs, external docs, data sources) | When you need domain knowledge            |
| `.claude/[freeform].md`      | Project-specific context docs (architecture, deployment, etc.) | As referenced from this file              |

Also: `PLAN.md` — architecture plan and potential future work

### Phase transitions

When a phase is completed:

1. **Condense** — extract lasting decisions from the active phase file and add to "Decisions from previous phases". Keep each to 1-2 lines.
2. **Archive** — remove the `current.md` symlink. The completed phase file stays but is no longer committed.
3. **Start fresh** — create a new numbered phase file from `~/.claude/phase-template.md`, then symlink `current.md` → it.
4. **Update this file** — update the "Current Phase" section above.
5. **Prune** — remove anything from this file that was phase-specific and no longer applies.
