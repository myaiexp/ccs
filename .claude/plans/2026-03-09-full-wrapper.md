# CCS Full Claude Wrapper — Implementation Plan

**Goal:** Make ccs the primary entry point for all claude interactions by removing inline mode, adding live tmux pane capture, and implementing a follow/split-pane view for monitoring active sessions.

**Architecture:** Remove tea.Exec code paths so ccs never suspends. Add `tmux capture-pane` polling for live terminal output of claude sessions. Introduce a follow mode that shows a split view (session list + live pane output). Enrich session metadata with tmux window IDs.

**Tech Stack:** Go, bubbletea, lipgloss, tmux capture-pane CLI

---

### Task 1: Remove inline launch mode [Mode: Direct]

**Files:**
- Modify: `internal/tui/launch.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/keys.go`

**Contracts:**
- Delete `trackedCmd` struct and its `Run()` method (launch.go lines 24-60)
- Delete `LaunchResume()` and `LaunchNew()` functions (launch.go lines 62-92)
- Delete `ExecFinishedMsg` type (launch.go lines 15-16)
- Keep `TmuxLaunchResume`, `TmuxLaunchNew`, `TmuxSwitch`, `TmuxLaunchDoneMsg`, `TmuxSwitchDoneMsg`
- In model.go: remove `launching` field from Model struct
- In model.go Update(): remove ExecFinishedMsg handler (lines 137-142)
- In model.go Update(): LaunchResumeMsg/LaunchNewMsg handlers should always use Tmux variants (no hub/non-hub branch)
- In model.go handleKey(): remove 'o' key handler (inline launch)
- In model.go handleKey(): number keys 1-9 always use tmux path (remove non-hub branch)
- In model.go handleEnter(): always call handleHubEnter(), remove handleInlineEnter()
- Delete `handleInlineEnter()` function entirely
- In model.go View(): remove the empty-view-while-launching logic
- In keys.go: remove `Inline` key binding

**Test Cases:**
```go
// Verify no inline types remain (compilation test)
// Verify Enter always triggers tmux launch for inactive sessions
// Verify Enter switches to existing tmux window for active sessions
// Verify 1-9 keys use tmux launch path
```

**Constraints:**
- ccs must be running inside tmux (already enforced by Bootstrap)
- If somehow not in tmux, show error instead of panic

**Verification:**
Run: `go build -o /dev/null . && go test ./... -count=1`
Expected: Compiles cleanly, all tests pass

**Commit after passing.**

---

### Task 2: Add tmux pane capture [Mode: Direct]

**Files:**
- Create: `internal/capture/capture.go`
- Create: `internal/capture/capture_test.go`
- Modify: `internal/tmux/tmux.go` (add CapturePaneContent)

**Contracts:**
```go
// internal/tmux/tmux.go — new function
func CapturePaneContent(windowID string, lines int) (string, error)
// Runs: tmux capture-pane -t <windowID> -p -S -<lines>
// Returns raw terminal output, trims trailing empty lines

// internal/capture/capture.go
package capture

type PaneSnapshot struct {
    SessionID string
    WindowID  string
    Content   string
    CapturedAt time.Time
}

// CapturePane captures the last N lines of a tmux window's visible output.
// Returns a PaneSnapshot or error if the window doesn't exist.
func CapturePane(sessionID, windowID string, lines int) (PaneSnapshot, error)
```

**Test Cases:**
```go
func TestCapturePaneContent_InvalidWindow(t *testing.T) {
    _, err := tmux.CapturePaneContent("@99999", 20)
    assert.Error(t, err)
}

func TestPaneSnapshot_Fields(t *testing.T) {
    snap := PaneSnapshot{SessionID: "abc", WindowID: "@1", Content: "hello"}
    assert.Equal(t, "abc", snap.SessionID)
}
```

**Constraints:**
- Lines parameter defaults to 30 if 0
- Trim trailing blank lines from capture output
- Return error if window doesn't exist (don't crash)

**Verification:**
Run: `go build -o /dev/null . && go test ./... -count=1`
Expected: Compiles, tests pass

**Commit after passing.**

---

### Task 3: Integrate pane capture into TUI model [Mode: Delegated]

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/styles.go`

**Contracts:**
- New Model fields:
  ```go
  followID      string                        // Session ID being followed (empty = normal)
  paneContent   map[string]capture.PaneSnapshot // SessionID → latest snapshot
  ```
- New message types:
  ```go
  PaneCaptureMsg struct { Snapshot capture.PaneSnapshot; Err error }
  PaneCaptureTickMsg struct{}
  ```
- New tea.Cmd: `paneCaptureCmd(sessionID, windowID string, lines int) tea.Cmd`
  - Calls `capture.CapturePane()`, returns `PaneCaptureMsg`
- New tea.Cmd: `paneCaptureTickCmd() tea.Cmd` — fires PaneCaptureTickMsg after 1s
- Update() handler for PaneCaptureMsg: store snapshot in `paneContent` map
- Update() handler for PaneCaptureTickMsg: if followID is set, dispatch paneCaptureCmd for the followed session, re-subscribe to tick
- Init paneContent as empty map in NewModel

**Constraints:**
- Only poll when followID is non-empty
- Capture 30 lines by default
- Don't poll for sessions without a tmux window ID

**Verification:**
Run: `go build -o /dev/null . && go test ./... -count=1`
Expected: Compiles, tests pass

**Commit after passing.**

---

### Task 4: Follow mode view with split pane [Mode: Delegated]

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/styles.go`

**Contracts:**
- `f` key on an active session with a tmux window → sets `followID`, starts pane capture tick
- `f` on already-followed session or `Esc` while following → clears `followID`, stops polling
- View() in follow mode renders split layout:
  - Top portion (~40%): compressed session list (fewer visible rows)
  - Bottom portion (~60%): bordered pane with live capture content for followed session
  - Header in bottom pane: session title + project name
- View() normal mode: unchanged (existing behavior)
- New key binding in keys.go: `Follow` key (`f`)

**Test Cases:**
```go
func TestFollowMode_Toggle(t *testing.T) {
    m := newTestModel()
    m.sessions = []types.Session{{ID: "s1", ActiveSource: types.SourceTmux}}
    // Pressing f sets followID
    // Pressing f again or Esc clears it
}

func TestFollowMode_RequiresActiveSession(t *testing.T) {
    // f on inactive session does nothing
}
```

**Constraints:**
- Follow mode only works on sessions with ActiveSource == SourceTmux (have tmux window)
- Attempting to follow a SourceProc or inactive session shows brief error message
- Split proportions: top gets enough rows for ~5-8 sessions, bottom gets the rest
- Bottom pane content scrolls to show latest output (bottom-anchored)

**Verification:**
Run: `go build -o /dev/null . && go test ./... -count=1`
Manual: Launch ccs, start a session, press `f` — should see split view with live capture

**Commit after passing.**

---

### Task 5: Enhanced detail pane with pane capture [Mode: Direct]

**Files:**
- Modify: `internal/tui/model.go`

**Contracts:**
- When detail pane is open (Enter on selected) for an active SourceTmux session:
  - Left column: existing session metadata (unchanged)
  - Right column: live pane capture content instead of JSONL activity entries
- For SourceProc sessions: keep existing JSONL activity in right column (fallback)
- For inactive sessions: single-column layout unchanged
- Start pane capture polling when detail pane opens on a SourceTmux session
- Stop polling when detail pane closes

**Constraints:**
- Reuse the same `paneCaptureCmd` and `PaneCaptureMsg` from Task 3
- Don't conflict with follow mode — if followID is set, detail pane capture uses same data

**Verification:**
Run: `go build -o /dev/null . && go test ./... -count=1`
Manual: Open detail pane on active tmux session — should show live terminal output

**Commit after passing.**

---

### Task 6: Status bar improvements [Mode: Direct]

**Files:**
- Modify: `internal/tui/model.go` (footer/status rendering)
- Modify: `internal/tui/styles.go` (status bar styles)

**Contracts:**
- Footer shows count of running instances: e.g., "3 active" next to existing footer content
- Per-session row: show last tool used (from activity entries) when available
- Follow mode footer: show "Following: <title> | f: unfollow | Esc: exit"

**Constraints:**
- Keep footer concise — don't overflow terminal width
- Active count comes from tracker.OpenSessionIDs() length

**Verification:**
Run: `go build -o /dev/null . && go test ./... -count=1`
Manual: Verify footer shows active count, follow mode shows correct hints

**Commit after passing.**

---

### Task 7: Update help overlay and key bindings [Mode: Direct]

**Files:**
- Modify: `internal/tui/model.go` (help overlay content)
- Modify: `internal/tui/keys.go` (add Follow key, remove Inline key)

**Contracts:**
- Help overlay (`?`) reflects new keybindings:
  - `f` — follow active session (split pane)
  - `Esc` — exit follow mode
  - Remove `o` (inline launch) from help
- Key descriptions updated

**Verification:**
Run: `go build -o /dev/null . && go test ./... -count=1`
Manual: Press `?` in ccs — help should show updated bindings

**Commit after passing.**

---

## Execution
**Skill:** superpowers:subagent-driven-development
- Mode A tasks (Direct): Tasks 1, 2, 5, 6, 7 — Opus implements directly
- Mode B tasks (Delegated): Tasks 3, 4 — Dispatched to subagents
