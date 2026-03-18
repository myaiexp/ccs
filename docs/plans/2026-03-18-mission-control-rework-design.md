# CCS Mission Control Rework — Design

## Problem

ccs is a session browser optimized for finding and resuming individual sessions from a flat list of 449+. But the actual workflow is **monitoring concurrent active sessions** and **tracking open work across projects**. The current layout (session list + project grid) doesn't serve either need well:

- Active sessions are mixed into hundreds of inactive ones, distinguished only by a colored dot
- No way to mark sessions as "done" — the list grows forever
- No organizational state — after a restart, there's no way to see "what was I working on?"
- Session names default to truncated first messages, which are rarely useful
- The project grid takes permanent screen space but is rarely used

## Goals

1. **Mission control** — tab into ccs and immediately see what all active sessions are doing
2. **Work tracking** — see open work items, mark things done, understand state after a restart
3. **Meaningful names** — sessions auto-named by what they accomplish, not their first message
4. **Noise reduction** — 440+ old sessions hidden by default, only active + open visible

## Non-goals

- Faster session launching (the `cd && claude` workflow is fine for starting work)
- Session search improvements beyond what's needed to replace the project grid
- Persistent pane capture across restarts
- Custom PTY multiplexing

---

## Architecture

### Data Model

**New file: `~/.cache/ccs/state.json`**

Stores session lifecycle state and ccs-owned metadata. Separate from `config.toml` (preferences) and `active.json` (runtime tracker).

```json
{
  "sessions": {
    "session-uuid": {
      "status": "open",
      "name": "config-sync autopull setup",
      "name_source": "auto",
      "completed_at": null
    }
  }
}
```

Fields:
- `status`: `"open"` or `"done"`. Active is computed from tracker, not stored.
- `name`: ccs-owned display name (3-6 words).
- `name_source`: `"auto"` (haiku-generated) or `"manual"` (user-typed). Manual names are never overwritten by auto-naming.
- `completed_at`: RFC3339 timestamp when marked done, null otherwise.

**Merged session state** — the TUI sees a single `StateStatus` per session:
- `Active` — tracker says PID is alive (computed, not stored)
- `Open` — in state.json with status "open", not currently active
- `Done` — in state.json with status "done"
- `Untracked` — not in state.json at all (the 440+ old sessions)

### New Package: `internal/state`

```go
type SessionState struct {
    Status      string     // "open" or "done"
    Name        string
    NameSource  string     // "auto" or "manual"
    CompletedAt *time.Time
}

type State struct {
    mu       sync.Mutex
    Sessions map[string]SessionState
    path     string
}

func Load() *State
func (s *State) Get(id string) (SessionState, bool)
func (s *State) Set(id string, state SessionState)
func (s *State) MarkOpen(id string)
func (s *State) MarkDone(id string)
func (s *State) Reopen(id string)
func (s *State) SetName(id, name, source string)
func (s *State) save()
```

Mutex-protected, saves on every mutation (same pattern as tracker).

### Session Lifecycle

```
                    ┌──────────────┐
                    │  Untracked   │ (not in state.json)
                    │  440+ old    │
                    └──────┬───────┘
                           │ tracker detects active
                           │ (auto-promote)
                           ▼
┌──────────┐       ┌──────────────┐
│  Active   │◄─────│    Open      │
│ (computed)│─────►│  (persisted) │
└──────────┘       └──────┬───────┘
  PID alive              │ user presses 'c'
  PID dies ──► stays     │
               Open      ▼
                    ┌──────────────┐
                    │    Done      │
                    │  (persisted) │
                    └──────┬───────┘
                           │ user presses 'o'
                           │ (reopen)
                           ▼
                    ┌──────────────┐
                    │    Open      │
                    └──────────────┘
```

- When tracker detects a new active session with no state.json entry → auto-promote to `open`
- When an active session's PID dies → stays `open` (user explicitly marks `done`)
- `c` key → marks `done`, sets `completed_at`
- `o` key → reopens to `open`, clears `completed_at`

### Types Changes

Add to `types.Session`:

```go
type StateStatus int

const (
    StatusUntracked StateStatus = iota
    StatusDone
    StatusOpen
    StatusActive
)
```

The `StateStatus` field is computed by merging tracker state (active/inactive) with state.json (open/done/absent). Set during refresh, consumed by the TUI for layout and rendering.

---

## Layout

### Three-Section View

```
ccs                                                          sort: time ↓

ACTIVE (3)
● piclaw      auth middleware migration    Editing internal/auth.go     71%  3m
  145 lines — within range. Let me verify...

● ccs         desloppify pipeline          Running go test ./...        94%  12m
  ok  ccs/internal/session (0.12s)

● investiq    i18n fix + deploy            ⏳ Waiting for input          50%  44m

OPEN (4)
┌─────────────────────────────────────────────────────────────────────────┐
│ piclaw      telegram bot transition                        103%  2d    │
│ /home/mse/Projects/piclaw  174 msgs  1.6 MB                           │
│ ○ inactive                                                            │
│                                                                       │
│ Running wc -l /home/mse/Projects/piclaw/CLAUDE.md                     │
│ Let me verify the final doc looks clean:                              │
└─────────────────────────────────────────────────────────────────────────┘
  central-hub dark mode v2                                    85%  2d
  mase.fi     20x plan update                                 85%  3d

412 done · h to show              c complete · R rename · / search · ? help · q quit
```

**Active section:**
- Always visible at top, never scrolled past
- Each active session gets 2-3 lines: header (project, name, live status, ctx%, time) + 1-2 lines of pane capture
- Live status derived from pane capture (see Attention States)
- Sorted by most recent activity first
- Not directly navigable with j/k — it's a dashboard. Interact via `enter` (switch), `f` (follow), or number shortcuts.

**Open section:**
- Scrollable, this is where j/k navigation lives
- Selected session shows detail pane (as current)
- Detail pane grows to fill available vertical space (no fixed height)
- Sorted by last active time (default), cycleable with `s`

**Done/untracked (via `h` toggle):**
- Appended below open, rendered dimmed
- Navigable when visible

**Footer:**
- Shows count of done sessions
- Context-sensitive key hints

### Project Grid Removed

The project grid is deleted entirely. Projects are accessible only through `/` search, which scans `~/Projects/*/` directories. This removes `tab` navigation, `left/right` keys, `n` key, and all grid layout code.

---

## Auto-Naming

### Invocation

```bash
echo "<prompt>" | claude --print --model haiku --no-session-persistence
```

- Uses existing Claude subscription, no API key needed
- `--no-session-persistence` prevents JSONL file creation
- Runs async in a goroutine, result delivered as `AutoNameMsg{SessionID, Name}`
- Cold start latency (~5-15s) is acceptable — naming is background work

### Trigger Points

1. **Session promoted to open** — when tracker first detects a session as active and it has no state.json entry. Fires once with available pane content.
2. **Session goes inactive** — when a previously active session's PID dies. Fires once with last pane snapshot. Overwrites the auto-name from trigger 1 (the "final summary" is usually better).
3. **Manual trigger** — `N` key on any selected session. Re-runs auto-naming.

Auto-naming only runs when `name_source != "manual"`. Manual names (set via `R`) are never overwritten.

### Prompt

```
Name this Claude Code session based on the terminal output below.
Reply with ONLY a 3-6 word task description (e.g. "config-sync autopull setup").
Focus on what task the session is accomplishing, not specific tools or code details.
If there isn't enough context, reply with exactly SKIP.

Terminal output:
{last N lines of pane capture or JSONL tail}
```

N is controlled by the `auto_name_lines` preference (default 20, cycleable: 10/20/30/50).

### Display Name Fallback Chain

1. Manual name (`name_source: "manual"`) — highest priority
2. Auto name (`name_source: "auto"`)
3. Claude's `/session-name` (parsed from JSONL `SessionName` field)
4. First user message title (current behavior)

---

## Attention States

Active sessions show a one-line live status derived from pane capture content. Computed every capture tick (1s).

### Status Detection

New function: `capture.DeriveStatus(snapshot PaneSnapshot) string`

Scans pane content bottom-up:

| State      | Detection                                              | Display                    |
| ---------- | ------------------------------------------------------ | -------------------------- |
| Waiting    | Line starting with `❯` or `$` at bottom of pane       | `⏳ Waiting for input`      |
| Permission | Line containing "Allow"/"Deny"/"permission" patterns   | `🔒 Permission prompt`     |
| Thinking   | Spinner line detected (`isSpinnerLine()` — existing)   | `Thinking...`              |
| Error      | Lines with "Error:", "FAIL", "error:" in recent output | `Error: <detail>`          |
| Working    | Default — falls back to latest activity entry          | `Editing file.go`, etc.    |

Pattern matching only, no AI. Must be fast (runs every second).

---

## Search Rework

### Current

`/` fuzzy-searches visible sessions + project grid names.

### New

`/` searches all sessions (regardless of lifecycle state) plus all project directories.

**Sources:**
1. All sessions — matches against project name, session name (ccs-owned), title
2. All directories under `~/Projects/` — scanned at startup and on refresh

**Result display:**

```
/ config-sync
  ● machine-configs  config-sync autopull setup          50%  3h
  ○ machine-configs  config-sync stash-safe pulls        29%  105h
  ✓ machine-configs  fix config-sync push                85%  5d
  ▸ machine-configs  ~/Projects/machine-configs          (new session)
```

State badges: `●` active, `○` open, `✓` done, `·` untracked, `▸` project dir

**Actions:**
- `enter` on active session → switch to tmux window
- `enter` on inactive session → resume in new tmux window
- `enter` on project dir → launch new session there
- `esc` → clear search, return to normal view
- Session keys (`c`, `R`, etc.) work on selected search result

---

## Key Bindings

### New/Changed

| Key    | Action                                                            |
| ------ | ----------------------------------------------------------------- |
| `c`    | Complete — mark selected session as done                          |
| `o`    | Reopen — move done session back to open                           |
| `R`    | Rename — inline text input for manual session name                |
| `N`    | Auto-name — re-trigger haiku naming on selected session           |
| `h`    | Toggle showing done/untracked sessions                            |
| `j/k`  | Navigate within open section (active is display-only)             |
| `enter`| Active → switch to tmux window. Otherwise → resume in new window  |
| `f`    | Follow mode (active sessions only, as current)                    |
| `d`    | Delete JSONL file (with confirm, as current)                      |
| `/`    | Fuzzy search all sessions + project dirs                          |
| `1-9`  | Shortcuts for first 9 visible sessions (active + open combined)   |
| `s`    | Cycle sort within open section                                    |
| `r`    | Reverse sort direction                                            |
| `p`    | Preferences popup                                                 |
| `?`    | Help overlay                                                      |
| `q`    | Quit                                                              |

### Removed

| Key         | Was                            | Replaced by        |
| ----------- | ------------------------------ | ------------------ |
| `tab`       | Switch sessions ↔ projects     | No project grid    |
| `n`         | Jump to projects section       | `/` search         |
| `x`         | Hide/unhide session            | `c` (complete)     |
| `left/right`| Project grid navigation        | No project grid    |

---

## Config Changes

### `~/.config/ccs/config.toml`

**Added:**
- `auto_name_lines = 20` — lines fed to haiku for naming (cycle: 10/20/30/50)

**Removed:**
- `hidden_projects` — no project grid
- `project_name_max` — no project grid

**Kept:**
- `hidden_sessions` — manual override, mostly replaced by lifecycle
- `claude_flags` — for launching sessions
- `relative_numbers` — display preference
- `tmux_session_name` — tmux bootstrap
- `activity_lines` — detail pane content height

### Preferences Popup

1. Relative numbers — toggle
2. Activity lines — cycle 3/5/10/15
3. Auto-name lines — cycle 10/20/30/50

---

## File Impact

### New Files
- `internal/state/state.go` — session lifecycle state + names
- `internal/naming/naming.go` — haiku invocation for auto-naming

### Major Changes
- `internal/types/types.go` — add `StateStatus`, remove project grid types if any
- `internal/tui/model.go` — new layout model, remove project grid state, add state integration
- `internal/tui/views.go` — complete view rewrite: active section, open section, expanded rows
- `internal/tui/keys.go` — updated key bindings
- `internal/tui/launch.go` — minimal changes (launch logic stays the same)
- `internal/capture/capture.go` — add `DeriveStatus()` for attention states
- `internal/config/config.go` — add `auto_name_lines`, remove project grid prefs
- `internal/project/project.go` — simplify to just directory scanning for search

### Removed
- Project grid layout/rendering code in `views.go` and `model.go`
- `FocusProjects`, `projectIdx`, `filteredProj`, grid navigation, `computeGridLayout()`
- `hidden_projects` config handling

### Unchanged
- `internal/session/parse.go` — JSONL parsing stays the same
- `internal/session/cache.go` — caching stays the same
- `internal/session/tracker.go` — PID tracking stays the same (state.go consumes its output)
- `internal/tmux/tmux.go` — tmux wrapper stays the same
- `internal/watcher/watcher.go` — fsnotify watcher stays the same
- `internal/activity/activity.go` — activity parsing stays the same
