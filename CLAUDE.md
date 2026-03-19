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
  naming/naming.go         Haiku invocation: status summaries, condensed names, comprehensive summaries. Logged.
  project/project.go       ScanProjectDirs for ~/Projects/ directory listing
  tmux/tmux.go             tmux CLI wrapper: Bootstrap, NewWindow, SelectWindow, WindowExists, CapturePaneContent
  capture/capture.go       PaneSnapshot, CapturePane, DeriveStatus (attention state detection)
  activity/activity.go     JSONL activity extraction: ExtractFromLine, TailFile, FormatEntry, ExtractConversationText
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
- **AI status summaries** — periodic (every 2 min) haiku-generated one-line summaries for active sessions. Up to 3 displayed with fading colors (newest brightest). Active status lines are dynamically capped by `maxActiveStatusLines()` based on terminal height. Content source: pane capture → JSONL conversation text fallback. `N` key triggers manually. Skips call if input unchanged. Logged to `~/.cache/ccs/naming.log`.
- **Transition summaries** — when session goes inactive, haiku condenses status history into a short name + comprehensive multi-line summary for the detail pane.
- **Display name fallback** — manual name > auto name > /session-name > first user message title
- **Attention states** — `DeriveStatus()` scans pane content bottom-up: waiting prompt, permission prompt, thinking/spinner, error, or fallback to last content line. Fast pattern matching (1s polling).
- **Stable active ordering** — active sessions don't re-sort on refresh; only new sessions insert at top. Prevents cursor disorientation.
- **Search rework** — `/` searches all sessions (any lifecycle state) + project directories at `~/Projects/`. Fuzzy matches filtered by score > 0 (eliminates noise). Results have scroll windowing with position indicator. Project dirs shown first for quick new-session launch. Results show state badges: `●` active, `○` open, `✓` done, `·` untracked, `▸` project dir.
- **tmux-only mode** — auto-bootstraps into tmux. All sessions open as new tmux windows.
- **Follow mode** — `f` key on active SourceTmux session enters split view with live pane capture.
- **Live pane capture** — 1s polling. Captures all sessions with tmux windows. Persists after inactive (dimmed).
- **Live activity monitoring** — fsnotify watches active JSONL files. 200ms debounce.
- **PID-based tracking** — `~/.cache/ccs/active.json` maps session IDs to PIDs and tmux window IDs.
- **JSONL parsing** streams line-by-line, never loads entire file into memory
- **Detail pane** — 2-column layout: left = AI comprehensive summary, right = JSONL conversation text. Right column: 2 sticky non-trivial (>20 char) user messages at top, then conversation tail (human `›` + assistant `»`, no tool calls). Text blocks collapsed to single lines. Right-side format: `time ctx%` with fixed-width fields for vertical alignment.
- **Height budgeting** — `maxActiveStatusLines()` dynamically caps status lines per active session based on terminal height. `scrollWindow()` fixedOverhead = 8 (border 2, title 1, OPEN header+margin 2, scroll indicator 1, footer+margin 2). Prevents top-row clipping when many active sessions are running.
- **Context window** — `maxContextTokens = 1000000` (Opus 4.6 1M context). Context % displayed right-aligned in fixed 4-char field.
- **Known gap**: pane capture for inactive sessions doesn't persist across ccs restarts (only in-memory).
- **Known gap**: pane capture frequently empty for active sessions (tracker doesn't always have tmux window IDs). JSONL conversation text fallback mitigates this.
- **lipgloss `.Width()` includes padding** — must subtract padding for text width calculations.
- **lipgloss doesn't clip content wider than `.Width()`** — all detail pane rows are hard-capped with `truncateToWidth(row, contentWidth)` to prevent border overflow.
- **lipgloss background on pre-styled text doesn't work** — inner ANSI resets cancel outer background. Use cursor indicators (`▸`) instead of background highlighting.

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
