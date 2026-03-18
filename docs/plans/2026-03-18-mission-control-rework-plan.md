# CCS Mission Control Rework — Implementation Plan

**Goal:** Redesign ccs from a session browser into a mission control dashboard with session lifecycle states (active/open/done), auto-naming via Haiku, active-first layout with live status, and search replacing the project grid.

**Architecture:** New `internal/state` package persists lifecycle + names to `~/.cache/ccs/state.json`. New `internal/naming` package shells out to `claude --print --model haiku` for auto-naming. TUI layout reworked to three sections (active/open/footer) with unified j/k navigation. Project grid replaced by `/` search that covers all sessions + project directories.

**Tech Stack:** Go 1.25+, bubbletea/lipgloss/bubbles, sahilm/fuzzy, fsnotify, BurntSushi/toml

**Spec:** `docs/plans/2026-03-18-mission-control-rework-design.md`

---

## Chunk 1: Foundation

### Task 1: State Package [Mode: A]

**Files:**
- Create: `internal/state/state.go`
- Test: `internal/state/state_test.go`

**Contracts:**
```go
type SessionState struct {
    Status      string     `json:"status"`       // "open" or "done"
    Name        string     `json:"name"`
    NameSource  string     `json:"name_source"`   // "auto" or "manual"
    CompletedAt *time.Time `json:"completed_at"`
}

type Store struct { /* mu sync.Mutex, sessions map[string]SessionState, path string */ }

func Load() *Store
func (s *Store) Get(id string) (SessionState, bool)
func (s *Store) Has(id string) bool
func (s *Store) MarkOpen(id string)
func (s *Store) MarkDone(id string)         // disallow if session is active (caller checks)
func (s *Store) Reopen(id string)
func (s *Store) SetName(id, name, source string)  // no-op if existing source is "manual" and new source is "auto"
func (s *Store) Remove(id string)                  // for cleaning up on session delete
func (s *Store) save()
```

Path: `os.UserCacheDir() + "/ccs/state.json"`. Mutex-protected, saves on mutation. Same pattern as `session/tracker.go`.

**Test Cases:**
- Load from missing file → empty store
- MarkOpen → Get returns status "open"
- MarkDone → sets CompletedAt, status "done"
- Reopen → clears CompletedAt, status "open"
- SetName with "manual" sticks; subsequent "auto" SetName is no-op
- SetName with "auto" on auto entry → overwrites
- Remove deletes entry
- JSON roundtrip preserves all fields
- Concurrent MarkOpen/MarkDone with `-race`

**Verification:** `go test ./internal/state/... -count=1 -race && go build .`

---

### Task 2: Types + Config Updates [Mode: A]

**Files:**
- Modify: `internal/types/types.go`
- Modify: `internal/config/config.go`
- Modify: `internal/project/project.go` (remove `HiddenProjects` usage)

**Changes to types.go:**

Add:
```go
type StateStatus int
const (
    StatusUntracked StateStatus = iota
    StatusDone
    StatusOpen
    StatusActive
)

// Add to Session struct:
StateStatus StateStatus
```

Update Config — add `AutoNameLines int`, remove `HiddenProjects []string` and `ProjectNameMax int`.

**Changes to config.go:** Update `applyDefaults` — add `AutoNameLines` default 20, remove `ProjectNameMax` default.

**Changes to project.go:** Remove `hiddenSet` / `HiddenProjects` logic from `DiscoverProjects`. Remove `Hidden` field from `types.Project`. Keep the function compiling — project grid isn't removed until Task 6.

**Fix all compilation errors** across the codebase from removed fields. Specific sites:
- `model.go` prefs handler: `case 2` cycles `ProjectNameMax` → replace with `AutoNameLines` cycle (10/20/30/50)
- `model.go` `renderPrefs`: items list references `ProjectNameMax` → replace with `AutoNameLines`
- `model.go` `computeGridLayout`: references `m.config.ProjectNameMax` → leave grid code intact for now (Task 6 removes it), but the field access must compile. Hardcode to 16 temporarily.
- `views.go` `renderProjects`: references `p.Hidden` → remove Hidden checks (field removed from Project)
- `model.go` `filterVisibleProjects`: references `p.Hidden` → simplify to pass-through
These are mechanical fixes to keep compiling — the actual layout rework is Task 6.

**Test Cases:**
- StateStatus.String() → "untracked"/"done"/"open"/"active"
- Config TOML roundtrip with `auto_name_lines`
- Config loads old file gracefully (defaults applied)

**Verification:** `go test ./... -count=1 && go build .`

---

### Task 3: DeriveStatus — Attention States [Mode: B]

**Files:**
- Modify: `internal/capture/capture.go`
- Create: `internal/capture/capture_test.go` (or add to existing)

**Contract:**
```go
func DeriveStatus(snap PaneSnapshot) string
```

Scans pane content bottom-up, returns first match:
1. Empty snapshot → `""`
2. Bottom line starts with `❯` or ends with `$ ` → `"Waiting for input"`
3. Recent lines contain "Allow"/"Deny" permission patterns → `"Permission prompt"`
4. Bottom has spinner (duplicate the small `isSpinnerLine` check, don't import tmux) → `"Thinking..."`
5. Recent lines match "Error:"/"FAIL"/"error:" → `"Error: <first ~50 chars>"`
6. Fallback → last non-empty non-spinner line, truncated to 60 chars

Compile regexes at package init. Must be fast (1s polling interval).

**Test Cases:** Empty, waiting prompt (`❯`), waiting prompt (`$`), permission, spinner, error, normal content, mixed cases.

**Verification:** `go test ./internal/capture/... -count=1 && go build .`

---

### Task 4: State Integration into Refresh Cycle [Mode: A]

**Files:**
- Modify: `main.go` — load `state.Load()`, pass to `tui.New`
- Modify: `internal/tui/model.go` — accept `*state.Store`, compute `StateStatus`, auto-promote, track prev-active set

**Changes to Model:**
```go
// New fields:
state         *state.Store
prevActiveIDs map[string]bool   // for detecting active→inactive transitions
```

**New in `New()` signature:** add `*state.Store` parameter.

**New function:**
```go
func computeStateStatuses(sessions []types.Session, tracker *session.Tracker, st *state.Store) {
    // For each session:
    // - if tracker says active → StatusActive, auto-promote to open if not in state
    // - else if state says "open" → StatusOpen
    // - else if state says "done" → StatusDone
    // - else → StatusUntracked
}
```

Called in `handleRefresh()` after `tracker.MarkActive()`.

**prevActiveIDs tracking:** Before computing new statuses, snapshot current active IDs. After refresh, compare: IDs in prev but not in new = "just went inactive". Store these for Task 5 (naming triggers).

**Test Cases:**
- Active session not in state → StatusActive + auto-promoted to open
- Already-open session becomes active → StatusActive (no double promote)
- Active session goes inactive → stays StatusOpen
- Done session → StatusDone regardless of activity
- Untracked session → StatusUntracked

**Verification:** `go test ./... -count=1 && go build .`

---

## Chunk 2: Auto-Naming

### Task 5: Naming Package + TUI Triggers [Mode: B]

**Files:**
- Create: `internal/naming/naming.go`
- Create: `internal/naming/naming_test.go`
- Modify: `internal/tui/model.go` — add AutoNameMsg, trigger points, `N` key

**Contracts in naming.go:**
```go
type Result struct {
    SessionID string
    Name      string  // empty on SKIP/error
    Err       error
}

func GenerateName(sessionID, contextText string, maxLines int) Result
```

Implementation: takes last `maxLines` lines of `contextText`, constructs prompt (from spec), pipes to `echo "..." | claude --print --model haiku --no-session-persistence` via `exec.CommandContext` with 60s timeout. Parses response: "SKIP" → empty, otherwise trim first line.

**TUI integration in model.go:**

New message:
```go
type AutoNameMsg struct { SessionID, Name string; Err error }
```

New messages:
```go
type AutoNameTriggerMsg struct { SessionID string }  // fired after 30s delay
```

Trigger points:
1. On auto-promote in `computeStateStatuses` → schedule `tea.Tick(30*time.Second, ...)` that fires `AutoNameTriggerMsg{sessionID}`. Handler dispatches the actual naming cmd.
2. On "just went inactive" (prevActiveIDs diff) → immediate naming cmd using `m.paneContent[id].Content`
3. `N` key → immediate naming cmd using JSONL tail content

Handler: on `AutoNameTriggerMsg` → dispatch `autoNameCmd` with content from `namingContent()`.
Handler: on `AutoNameMsg` → if name non-empty, call `m.state.SetName(id, name, "auto")`

**Content source for naming:**
```go
func (m *Model) namingContent(sessionID string) string {
    // Prefer pane capture if available
    if snap, ok := m.paneContent[sessionID]; ok && snap.Content != "" {
        return snap.Content
    }
    // Fallback: read last N lines of JSONL file directly (not activity.TailFile
    // which returns structured Entry objects). Use naming.TailFileLines(path, n)
    // which reads the raw file tail and returns plain text lines.
    for _, s := range m.sessions {
        if s.ID == sessionID && s.FilePath != "" {
            return naming.TailFileLines(s.FilePath, m.config.AutoNameLines)
        }
    }
    return ""
}
```

**Add to naming.go:**
```go
// TailFileLines reads the last N lines from a file as plain text.
// Used to provide JSONL content as context for naming when pane capture
// is not available. Returns raw text (not parsed entries).
func TailFileLines(path string, maxLines int) string
```

**Test Cases (naming.go):**
- "SKIP" response → empty name
- Normal response → trimmed first line
- Multiline response → first line only
- Prompt construction includes correct line count
- Missing `claude` binary → graceful error

**Verification:** `go test ./internal/naming/... -count=1 && go test ./... -count=1 && go build .`

---

## Chunk 3: TUI Rework

### Task 6: Layout — Active/Open Three-Section View [Mode: B]

**Files:**
- Modify: `internal/tui/model.go` — major rework
- Modify: `internal/tui/views.go` — major rewrite
- Modify: `internal/tui/styles.go` — add/remove styles
- Modify: `internal/tui/launch.go` — `TmuxLaunchNew` accepts dir path string

This is the largest task. Key changes:

**model.go removals:**
- `Focus` enum, `FocusSessions`/`FocusProjects`
- `projects`, `filteredProj`, `projectIdx`
- `computeGridLayout`, `projectGrid`, `gridPosition`, `gridLayout` type
- `filterVisibleProjects`
- `handleNavigation` cases for `tab`, `left`, `right`
- `showHidden` → rename to `showDoneUntracked`

**model.go additions/changes:**
- `sessionIdx` now navigates active+open as one continuous list (active first, then open)
- `applyFilter` rework: split sessions by StateStatus into display groups. No project filtering.
- New helper: `activeSessions()`, `openSessions()` computed from `m.filtered` by StateStatus
- `scrollWindow` rewrite: active section is always fully visible at top, open section scrolls below it
- `handleEnter` rework: check StateStatus for switch vs resume logic
- Remove `project.DiscoverProjects()` call from `handleRefresh()` and `m.projects` field
- Update `gg`/`G` bounds for the unified active+open list (and done/untracked when visible)

**views.go removals:**
- `renderProjects`

**views.go additions/changes:**
- `renderActiveRow(s Session)` — 2-3 lines: header (●, project, display name, DeriveStatus, ctx%, time) + 1-2 lines pane capture
- `renderSession` rework — now for open section only (compact, one line, with ○ badge)
- `renderDetail` — for selected open session, detail pane grows to fill space
- `View()` — three sections: active header+rows, open header+rows (with detail), footer with done count
- `formatDuration` — add day support (>= 24h)
- `displayName(s Session) string` — fallback chain: manual name > auto name > session name > title
- Done/untracked rendering when `showDoneUntracked` is true: appended below open, dimmed, with `✓`/`·` badges

**styles.go:**
- Remove: `selectedProjectStyle`, `normalProjectStyle`, `hiddenProjectStyle`
- Add: active row background/highlight, status text style, badge styles for `○`, `✓`, `·`

**launch.go:**
- `TmuxLaunchNew` → accept `dir string, name string` instead of `types.Project`

**Test Cases:**
- `formatDuration` with 25h → "1d", 49h → "2d", 168h → "7d"
- `displayName` fallback chain
- Active/open partitioning from filtered sessions
- Cursor navigation across active→open boundary
- `scrollWindow` with active rows + open rows

**Verification:** `go test ./... -count=1 && go build .`

---

### Task 7: Key Bindings — Complete/Reopen/Rename [Mode: A]

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/views.go`

**New key handlers:**
- `c` — if selected session `StatusActive` → error "session still running". If `StatusOpen` → `state.MarkDone(id)`. If already `StatusDone` → no-op.
- `o` — if `StatusDone` → `state.Reopen(id)`. Otherwise no-op.
- `R` — enter rename mode. New fields: `renaming bool`, `renameInput textinput.Model`, `renameTarget string`. Footer shows text input pre-filled with current display name. Enter → `state.SetName(id, value, "manual")`. Esc → cancel.

**Remove handlers:** `tab`, `n`, `x`, `left`, `right` (already gone from Task 6 model changes).

**Prefs popup update:** Remove project name max (index 2). Replace with auto-name lines cycle (10/20/30/50). Adjust `prefsCount` and switch cases.

**views.go:** Update `renderFooter` hints, `renderHelp` overlay text. Add rename input rendering when `m.renaming`.

**Test Cases:**
- `c` on active → error
- `c` on open → marks done
- `o` on done → reopens
- `o` on open → no-op
- Rename confirm saves manual name
- Rename cancel preserves old name

**Verification:** `go test ./... -count=1 && go build .`

---

### Task 8: Search Rework [Mode: B]

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/views.go`
- Modify: `internal/tui/launch.go`
- Modify: `internal/project/project.go`

**New types:**
```go
type SearchResult struct {
    Session *types.Session  // nil for project dir results
    DirPath string          // for project dirs
    DirName string          // display name
}
```

**project.go changes:** Add `ScanProjectDirs(root string) []ProjectDir` that scans the **actual project directories** at `~/Projects/` (NOT `~/.claude/projects/` which is the JSONL session source). Uses `os.ReadDir(root)` and returns `{Name, Path}` pairs. Old `DiscoverProjects` can be removed if no longer needed.

**model.go changes:**
- New field: `projectsRoot string` — `~/Projects/` path, passed from main.go
- New field: `projectDirs []project.ProjectDir` — scanned at startup and on refresh
- When `m.filtering`: build `[]SearchResult` from fuzzy matching against ALL sessions (including done/untracked) + project dirs
- New field: `searchResults []SearchResult`, `searchIdx int`
- `handleEnter` in search mode: session → switch/resume, dir → `TmuxLaunchNewDir`

**views.go:** When filtering, render search results with state badges: `●` active, `○` open, `✓` done, `·` untracked, `▸` project dir.

**launch.go:** Add `TmuxLaunchNewDir(name, dir string, flags []string, tracker)` — simpler than current `TmuxLaunchNew`.

**Lifecycle keys in search mode:** `c`, `o`, `R` operate on the selected search result if it's a session. This is owned by Task 8 — the key handlers from Task 7 already exist, but Task 8 must wire them to work on `searchResults[searchIdx].Session` when `m.filtering` is true.

**Test Cases:**
- Search matches by project name, display name, title
- Search matches project dir by name
- Enter on dir launches new session
- State badges render correctly
- `c`/`o`/`R` work on sessions in search results

**Verification:** `go test ./... -count=1 && go build .`

---

### Task 9: Cleanup + CLAUDE.md Update [Mode: A]

**Files:**
- All files — dead code removal
- `CLAUDE.md` — update to reflect new architecture
- `PLAN.md` — update current state

Remove any remaining dead code: unused `Project.Hidden`, `Focus` if still present, orphaned helpers, unused styles. Run `go vet ./...`. Update help overlay. Update CLAUDE.md key bindings, config, and structure sections.

**Verification:** `go vet ./... && go test ./... -count=1 -race && go build .`

---

## Execution
**Skill:** superpowers:subagent-driven-development
- Mode A tasks (1, 2, 4, 7, 9): Opus implements directly
- Mode B tasks (3, 5, 6, 8): Dispatched to subagents
