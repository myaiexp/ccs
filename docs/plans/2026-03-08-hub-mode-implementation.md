# Hub Mode Implementation Plan

**Goal:** Transform ccs from a launch-and-exit tool into a persistent tmux hub with live activity monitoring.

**Architecture:** ccs runs inside tmux, launches sessions as new tmux windows, watches active session JSONL files via inotify for real-time activity updates. Legacy inline mode remains via `o` keybind. Periodic 10s tick handles session discovery; inotify handles activity.

**Tech Stack:** Go 1.25+, bubbletea, fsnotify (new dep), tmux CLI via os/exec

---

### Task 1: tmux Module [Mode: Delegated]

**Files:**
- Create: `internal/tmux/tmux.go`
- Test: `internal/tmux/tmux_test.go`

**Contracts:**
```go
package tmux

func InTmux() bool                                                    // checks $TMUX env var
func Bootstrap(sessionName string) error                              // syscall.Exec into tmux new-session -s <name> ccs
func NewWindow(name, dir string, cmdAndArgs []string) (string, error) // tmux new-window -P -F '#{window_id}' -n <name> -c <dir> <cmd...>
func SelectWindow(windowID string) error                              // tmux select-window -t <windowID>
func WindowExists(windowID string) bool                               // tmux list-windows -F '#{window_id}', check membership
```

**Test Cases:**
```go
func TestInTmux_Set() {
    // Set $TMUX, verify InTmux() returns true
}
func TestInTmux_Unset() {
    // Unset $TMUX, verify InTmux() returns false
}
func TestNewWindow_CommandConstruction() {
    // Verify the args slice built for exec.Command without executing
    // (extract command-building into a testable helper)
}
```

**Constraints:**
- Use `os/exec` to call `tmux` binary, not a Go library
- `Bootstrap` replaces the process via `syscall.Exec` — never returns on success
- `NewWindow` uses `-P -F '#{window_id}'` to capture and return the window ID

**Verification:** `go test ./internal/tmux/ -count=1 -v`
**Commit after passing.**

---

### Task 2: Extend Tracker for tmux Window IDs [Mode: Direct]

**Files:**
- Modify: `internal/types/types.go`
- Modify: `internal/session/tracker.go`
- Test: `internal/session/tracker_test.go`

**Contracts:**

In `types.go` — add `ActiveSource` enum and field:
```go
type ActiveSource int
const (
    SourceInactive ActiveSource = iota
    SourceProc                          // found via /proc, no tmux window
    SourceTmux                          // launched from ccs, has tmux window
)
// Add ActiveSource field to Session struct alongside existing IsActive bool
```

In `tracker.go` — add `TmuxWindowID` to `TrackedSession`, new methods:
```go
type TrackedSession struct {
    // ... existing fields ...
    TmuxWindowID string `json:"tmux_window_id,omitempty"`
}

func (t *Tracker) FindBySessionID(id string) (TrackedSession, bool)
func (t *Tracker) TmuxWindowIDs() map[string]string
func (t *Tracker) SetTmuxWindow(sessionID, windowID string)
```

Extend `Refresh()` to also prune entries where `TmuxWindowID` is set but `tmux.WindowExists()` returns false.

**Test Cases:**
```go
func TestTracker_FindBySessionID() {
    // Track two sessions, find by ID, verify correct one returned
    // Find non-existent ID, verify false
}
func TestTracker_TmuxWindowIDs() {
    // Track sessions with and without tmux window IDs
    // Verify map only contains entries with window IDs
}
func TestTracker_SetTmuxWindow() {
    // Track a session, set its tmux window, verify it persists
}
```

**Constraints:**
- Keep `IsActive bool` field — set both `IsActive` and `ActiveSource` during marking phase in `handleRefresh()` and `main.go`
- Existing tests must pass unchanged (TmuxWindowID defaults to empty)

**Verification:** `go test ./internal/session/ -count=1 -v && go test ./internal/types/ -count=1 -v`
**Commit after passing.**

---

### Task 3: Activity Extraction from JSONL [Mode: Delegated]

**Files:**
- Create: `internal/activity/activity.go`
- Test: `internal/activity/activity_test.go`

**Contracts:**
```go
package activity

type Entry struct {
    Type      string    // "tool_use" or "text"
    Tool      string    // "Read", "Edit", "Bash", etc. Empty for text.
    Summary   string    // "Edit model.go", "Bash: go test ./...", "Fixed the import..."
    Timestamp time.Time
}

func ExtractFromLine(line []byte) []Entry    // parse single JSONL line, return nil if not assistant msg
func TailFile(path string, maxEntries int) []Entry  // seek to last ~32KB, extract latest activities
func FormatEntry(e Entry) string              // display string: "Editing model.go", "Running go test"
```

**Parsing logic for `ExtractFromLine`:**
- Only process lines where `type == "assistant"`
- Parse `message.content` as array of objects
- `type: "tool_use"` → extract `name` and `input` fields, build summary:
  - Read/Edit/Write: basename of `input.file_path`
  - Bash: first ~40 chars of `input.command`
  - Grep/Glob: `input.pattern` truncated
  - Others: just tool name
- `type: "text"` → first line, max ~60 chars

**`TailFile`:** seek to `max(0, filesize-32KB)`, read to EOF, split by newlines, discard first partial line, process each, return latest `maxEntries` (newest first).

**Test Cases:**
```go
func TestExtractFromLine_ToolUse() {
    // Feed JSONL with tool_use content block, verify Entry fields
}
func TestExtractFromLine_Text() {
    // Feed JSONL with assistant text, verify summary truncation
}
func TestExtractFromLine_IgnoresUserMessages() {
    // Feed user message line, verify nil return
}
func TestExtractFromLine_MultipleTools() {
    // Content array with 2 tool_use blocks, verify 2 entries returned
}
func TestFormatEntry_ToolUse() {
    // Verify "Editing model.go", "Running go test ./..."
}
func TestTailFile() {
    // Write temp JSONL file with multiple lines, verify extraction order and count
}
```

**Constraints:**
- Define own minimal JSON structs — don't couple to parse.go internals
- Must handle malformed lines gracefully (return nil, don't crash)
- TailFile must discard the first partial line after seeking

**Verification:** `go test ./internal/activity/ -count=1 -v`
**Commit after passing.**

---

### Task 4: Inotify File Watcher [Mode: Delegated]

**Files:**
- Create: `internal/watcher/watcher.go`
- Test: `internal/watcher/watcher_test.go`

**Dependency:** `go get github.com/fsnotify/fsnotify`

**Contracts:**
```go
package watcher

type ActivityUpdate struct {
    SessionID string
    FilePath  string
    Entries   []activity.Entry
}

type Watcher struct { /* fsnotify.Watcher, watched map, updates chan, config */ }

func New(activityLines int) (*Watcher, error)
func (w *Watcher) Watch(sessionID, filePath string) error
func (w *Watcher) Unwatch(filePath string)
func (w *Watcher) UnwatchAll()
func (w *Watcher) Run()                    // goroutine: reads fsnotify events, debounces, sends to channel
func (w *Watcher) WatchCmd() tea.Cmd       // blocks on channel, returns ActivityUpdateMsg
func (w *Watcher) Close()
```

**Key design:**
- `WatchCmd()` returns a `tea.Cmd` that blocks on a buffered channel (size ~100) — standard bubbletea async pattern
- Model re-issues `WatchCmd()` after each `ActivityUpdateMsg`
- `Run()` debounces by 200ms per file path before calling `activity.TailFile`
- Runs in a goroutine started from `Init()`

**Test Cases:**
```go
func TestWatcher_WatchUnwatch() {
    // Add and remove watches, verify internal state
}
func TestWatcher_FileModification() {
    // Create temp file, watch it, write to it, verify ActivityUpdate received on channel
}
```

**Constraints:**
- Buffered channel prevents TUI hang if events arrive faster than processed
- 200ms debounce per file path — JSONL gets rapid sequential writes during tool use
- Must handle watcher errors gracefully (log, don't crash)

**Verification:** `go test ./internal/watcher/ -count=1 -v`
**Commit after passing.**

---

### Task 5: tmux Launch Integration [Mode: Delegated]

**Files:**
- Modify: `internal/tui/launch.go` — add TmuxLaunchResume, TmuxLaunchNew, TmuxSwitch
- Modify: `internal/tui/model.go` — add `hubMode` field, change handleEnter, add `o` key handler, handle new msgs
- Modify: `internal/tui/keys.go` — add `Inline` key binding for `o`

**Contracts:**

New in `launch.go`:
```go
func TmuxLaunchResume(sess types.Session, flags []string, tracker *session.Tracker) tea.Cmd
func TmuxLaunchNew(proj types.Project, flags []string, tracker *session.Tracker) tea.Cmd
func TmuxSwitch(windowID string) tea.Cmd
```

New message types:
```go
type TmuxLaunchDoneMsg struct{ Err error }
type TmuxSwitchDoneMsg struct{ Err error }
```

**Model changes:**
- Add `hubMode bool` to Model, set in `New()` via `tmux.InTmux()`
- `handleEnter()`:
  1. hubMode + session has tmux window → `TmuxSwitch(windowID)`
  2. hubMode + session exists → `TmuxLaunchResume`
  3. hubMode + project → `TmuxLaunchNew`
  4. !hubMode → current tea.Exec behavior
- `o` key → always legacy inline (current LaunchResume/LaunchNew)
- Number shortcuts `1-9` follow same hub/legacy logic as `enter`
- Hub mode launches don't set `m.launching = true` (TUI stays visible)

**Constraints:**
- `TmuxLaunchResume` creates window with name `"proj: title"` truncated to ~30 chars
- After tmux launch, return `RefreshMsg{}` (not `ExecFinishedMsg`)
- Must record tmux window ID in tracker via `tracker.SetTmuxWindow()`

**Verification:** `go build -o /dev/null .`
**Commit after passing.**

---

### Task 6: Session Row Layout and Three-State Dot [Mode: Delegated]

**Files:**
- Modify: `internal/tui/styles.go` — add externalDot (yellow), activityStyle
- Modify: `internal/tui/model.go` — renderSession(), renderDetail() two-column layout

**Contracts:**

New styles:
```go
externalDot   // yellow "●" (color 220)
activityStyle // dim italic (color 245)
```

Add to Model:
```go
activities map[string][]activity.Entry // sessionID -> recent entries
```

**renderSession() changes:**
- Dot: `SourceTmux` → green, `SourceProc` → yellow, `SourceInactive` → dim
- Row layout: `dot num project first-message...  activity  ctx% time`
- First message fills available space (fix current aggressive truncation)
- Active sessions: latest activity summary right-aligned before ctx%, rendered with `activityStyle`
- Inactive sessions: first message fills that space

**renderDetail() changes:**
- Active sessions: two-column layout using `lipgloss.JoinHorizontal`
  - Left (~40%): existing info (project dir, messages/size/status, ID, first message wrapped)
  - Right (~60%): activity log entries formatted with `activity.FormatEntry()`
  - Height: `max(leftLines, activityLines)`
- Inactive sessions: current single-column layout unchanged

**Constraints:**
- Activity lines count comes from `m.config.ActivityLines` (default 5)
- Detail pane height must adjust dynamically — update `detailPaneLines()` accordingly
- Two-column layout only when session is active AND has activity entries

**Verification:** `go build -o /dev/null .`
**Commit after passing.**

---

### Task 7: Wire Up Watcher and Periodic Refresh [Mode: Direct]

**Files:**
- Modify: `internal/tui/model.go` — Init(), Update() for ActivityUpdateMsg and TickMsg, watcher lifecycle
- Modify: `main.go` — create watcher, pass to Model

**Contracts:**

New message types:
```go
type ActivityUpdateMsg watcher.ActivityUpdate
type TickMsg struct{}
```

Model additions:
```go
type Model struct {
    // ... existing ...
    watcher    *watcher.Watcher
    activities map[string][]activity.Entry
}
```

**Init():** return `tea.Batch(tickCmd(10s), watcher.WatchCmd())`, watch all currently-active sessions.

**Update():**
- `ActivityUpdateMsg` → update `m.activities[msg.SessionID]`, re-issue `watcher.WatchCmd()`
- `TickMsg` → call `handleRefresh()`, compare old vs new active sets to Watch/Unwatch files, re-issue `tickCmd(10s)`

**main.go:** create watcher with `watcher.New(cfg.ActivityLines)`, pass to `tui.New()`

**Constraints:**
- On refresh, diff active session set: watch newly-active files, unwatch newly-inactive
- Watcher.Close() on quit

**Verification:** `go build -o ~/.local/bin/ccs . && go test ./... -count=1`
**Commit after passing.**

---

### Task 8: Config, Prefs, Footer, Bootstrap [Mode: Direct]

**Files:**
- Modify: `internal/types/types.go` — add TmuxSessionName, ActivityLines to Config
- Modify: `internal/tui/model.go` — renderPrefs(), renderFooter(), renderHelp()
- Modify: `main.go` — tmux bootstrap before session discovery

**Contracts:**

Config additions:
```go
type Config struct {
    // ... existing ...
    TmuxSessionName string `toml:"tmux_session_name"` // default: "ccs"
    ActivityLines   int    `toml:"activity_lines"`     // default: 5
}
```

Defaults: apply after `config.Load()` — if empty/zero, set to `"ccs"` / `5`.

**main.go bootstrap:**
```go
if !tmux.InTmux() {
    tmux.Bootstrap(cfg.TmuxSessionName) // replaces process, never returns on success
}
```

**Footer:** hub mode shows `"enter switch/resume  o inline"` instead of `"enter resume"`.

**Prefs:** add activity_lines as cycle option (3, 5, 10, 15).

**Help:** add `o` key line, update `enter` description.

**Verification:** `go build -o ~/.local/bin/ccs . && go test ./... -count=1`
**Commit after passing.**

---

## Execution Order

```
Task 1 (tmux module) ──────┐     Task 3 (activity parsing) ─────┐
                            ▼                                     ▼
                     Task 2 (tracker)                      Task 4 (watcher)
                            │                                     │
                            ▼                                     │
                     Task 5 (tmux launch)    Task 6 (row layout) ◄┘
                            │                       │
                            └──────┬────────────────┘
                                   ▼
                            Task 7 (wire up)
                                   │
                                   ▼
                            Task 8 (config/polish)
```

Parallelizable: 1+3, then 2+4, then 5+6, then 7, then 8.

---

## Verification

End-to-end test after all tasks:
1. Run `ccs` outside tmux → should auto-bootstrap into tmux session
2. Select a session, press `enter` → new tmux window opens with claude
3. Switch back to ccs tab → session shows green dot and live activity
4. Press `enter` on same session → switches to existing tmux window (no duplicate)
5. Press `o` on a session → legacy inline mode (TUI suspends)
6. Open detail pane → two-column layout with activity log on right
7. Kill a tmux window externally → dot turns dim on next refresh
8. Check prefs → activity_lines option present and functional

---
## Execution
**Skill:** superpowers:subagent-driven-development
- Mode A tasks: Opus implements directly (Tasks 2, 7, 8)
- Mode B tasks: Dispatched to subagents (Tasks 1, 3, 4, 5, 6)
