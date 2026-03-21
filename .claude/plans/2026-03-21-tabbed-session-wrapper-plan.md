# Tabbed Session Wrapper — Implementation Plan

**Goal:** Transform ccs from a session monitor into a tabbed session manager where Claude sessions are tmux windows managed by ccs, with a status line tab bar and attention system.

**Architecture:** ccs manages tmux windows within its session — dashboard in window 0, Claude sessions in subsequent windows. A tab manager orchestrates window lifecycle (launch, adopt, exit detection). The tmux status line renders a tab bar with attention states. A unix socket enables `ccs launch` from external tools (Kelo). The bubbletea TUI simplifies to a dashboard-only role.

**Tech Stack:** Go 1.25+, bubbletea/lipgloss, tmux 3.4+ (status-format, window options, hooks)

**Spec:** `.claude/plans/2026-03-21-tabbed-session-wrapper-design.md`

---

### Task 1: tmux package — window & session management primitives [Mode: Direct]

Add thin wrapper functions to the tmux package for window management, status line control, keybinding management, and hooks. All functions are stateless CLI wrappers.

**Files:**
- Modify: `internal/tmux/tmux.go`
- Test: `internal/tmux/tmux_test.go`

**Contracts:**

```go
// MoveWindow moves a tmux window into the target session.
// Uses -d to avoid focus switch, trailing colon auto-assigns index.
func MoveWindow(windowID, targetSession string) error

// SetWindowOption sets a user option on a tmux window (@ prefix).
func SetWindowOption(windowID, key, value string) error

// GetWindowOption gets a user option from a tmux window. Returns "" if unset.
func GetWindowOption(windowID, key string) string

// RenameWindow changes a tmux window's display name.
func RenameWindow(windowID, name string) error

// SetStatusFormat sets the tmux status line format for the current session.
// lineIndex: 0 or 1 (for two-line status). Uses session-scoped -s option.
func SetStatusFormat(lineIndex int, format string) error

// UnsetStatusFormat removes session-scoped status format overrides.
func UnsetStatusFormat() error

// SetStatusLines sets the number of status lines (1 or 2). Session-scoped.
func SetStatusLines(count int) error

// ListKeys returns current keybindings for the given key table.
// Returns raw output lines from `tmux list-keys -T <table>`.
func ListKeys(table string) ([]string, error)

// BindKey registers a tmux keybinding in the given table.
func BindKey(table, key, command string) error

// UnbindKey removes a tmux keybinding.
func UnbindKey(table, key string) error

// SetHook registers a window-scoped tmux hook (-w -t <windowID>).
// hookName: e.g. "pane-exited". command: shell command to run.
func SetHook(windowID, hookName, command string) error

// RemoveHook removes a window-scoped tmux hook.
func RemoveHook(windowID, hookName string) error

// CurrentWindowID returns the currently focused window ID.
// Uses `list-windows -F '#{window_active} #{window_id}'` filtered for active=1.
func CurrentWindowID() (string, error)

// AllPanesBySession returns all pane PIDs grouped by tmux session name and window ID.
// Used by ScanAndAdopt to find Claude processes outside the ccs session.
func AllPanesBySession() (map[string]map[string]int, error) // session → windowID → PID

// SessionWindows returns all window IDs in the given tmux session.
func SessionWindows(sessionName string) ([]string, error)

// CurrentSessionName returns the name of the current tmux session.
func CurrentSessionName() (string, error)
```

**Test Cases:**

```go
func TestMoveWindowCommand(t *testing.T) {
    // Verify MoveWindow builds correct tmux command args
}

func TestGetWindowOptionEmpty(t *testing.T) {
    // GetWindowOption returns "" for unset options (not an error)
}

func TestSetStatusFormatLineIndex(t *testing.T) {
    // SetStatusFormat(0, ...) targets status-format[0]
    // SetStatusFormat(1, ...) targets status-format[1]
}

func TestCurrentWindowIDFiltersActive(t *testing.T) {
    // Verifies parsing of list-windows output to find active=1 entry
}

func TestAllPanesBySessionGrouping(t *testing.T) {
    // Verifies correct grouping of panes by session and window
}
```

**Constraints:**
- All functions are pure tmux CLI wrappers — no state, no side effects beyond the tmux call
- Error handling: return exec errors directly, callers decide policy
- `SetWindowOption` uses `@` prefix for user options per tmux convention
- `SetHook` uses `tmux set-hook -w -t <windowID>` for per-window hooks

**Verification:**
Run: `go test ./internal/tmux/ -v -count=1`
Expected: All tests pass

**Commit after passing.**

---

### Task 2: IPC package + `ccs launch` CLI [Mode: Delegated]

New package for unix socket IPC between the running ccs instance and external callers. Plus CLI subcommand parsing in main.go.

**Files:**
- Create: `internal/ipc/protocol.go`
- Create: `internal/ipc/server.go`
- Create: `internal/ipc/client.go`
- Modify: `main.go`
- Test: `internal/ipc/ipc_test.go`

**Contracts:**

```go
// protocol.go — request/response types

// Message wraps all IPC messages with a type discriminator.
type Message struct {
    Type string          `json:"type"` // "launch", "exit"
    Data json.RawMessage `json:"data"`
}

type LaunchRequest struct {
    ProjectDir string `json:"project_dir"`
    ResumeID   string `json:"resume_id,omitempty"`
    Prompt     string `json:"prompt,omitempty"`
    OnDone     string `json:"on_done,omitempty"`
}

type LaunchResponse struct {
    OK        bool   `json:"ok"`
    SessionID string `json:"session_id,omitempty"`
    Error     string `json:"error,omitempty"`
}

// ExitNotification is sent by the pane-exited hook to signal session completion.
type ExitNotification struct {
    WindowID string `json:"window_id"`
}

// server.go — listens on unix socket, dispatches commands

type Handler struct {
    OnLaunch func(req LaunchRequest) LaunchResponse
    OnExit   func(notif ExitNotification)
}

type Server struct { ... }

// NewServer creates a server on the given socket path.
// Handles stale socket: attempts net.Listen, if file exists but no listener, removes and retries.
func NewServer(socketPath string) (*Server, error)

// SetHandler registers callbacks for IPC messages.
func (s *Server) SetHandler(h Handler)

// Serve starts accepting connections. Blocks. Call in a goroutine.
func (s *Server) Serve() error

// Close stops the server and removes the socket file.
func (s *Server) Close() error

// SocketPath is the default socket location.
const SocketPath = "~/.cache/ccs/ccs.sock" // expanded at runtime

// client.go — connects to running ccs instance

// Launch sends a launch request to the running ccs instance.
func Launch(socketPath string, req LaunchRequest) (LaunchResponse, error)

// NotifyExit sends an exit notification to the running ccs instance.
// Used by the pane-exited tmux hook: `ccs notify-exit --window @42`
func NotifyExit(socketPath string, notif ExitNotification) error
```

```go
// main.go changes — subcommand parsing before TUI startup
//
// If os.Args[1] == "launch" → parse flags, call ipc.Launch(), exit
// If os.Args[1] == "notify-exit" → parse --window, call ipc.NotifyExit(), exit
// Otherwise → normal TUI startup
```

**Test Cases:**

```go
func TestServerClientLaunchRoundtrip(t *testing.T) {
    // Start server, send LaunchRequest via client, verify handler called + response returned
}

func TestServerExitNotification(t *testing.T) {
    // Send ExitNotification via client, verify OnExit handler called with correct WindowID
}

func TestClientNoServer(t *testing.T) {
    // Client connecting to nonexistent socket returns clear error
}

func TestStaleSocketCleanup(t *testing.T) {
    // Create a socket file (no listener), verify NewServer removes it and starts fresh
}

func TestServerCloseRemovesSocket(t *testing.T) {
    // After server.Close(), socket file should not exist
}

func TestConcurrentLaunches(t *testing.T) {
    // Multiple clients sending launch requests concurrently all get responses
}
```

**Constraints:**
- Protocol: one JSON `Message` per connection (connect → send → read response if applicable → close)
- `ExitNotification` is fire-and-forget (no response expected)
- `ccs launch` without a running ccs exits with error: "ccs is not running — start ccs first"
- `ccs launch --project` resolves relative paths to absolute before sending
- `ccs notify-exit` is called by tmux hooks — must be fast and non-blocking

**Verification:**
Run: `go test ./internal/ipc/ -v -count=1 && go build -o /dev/null .`
Expected: All tests pass, build succeeds with new main.go

**Commit after passing.**

---

### Task 3: Tab manager + status line renderer [Mode: Delegated]

New package that orchestrates session windows — launch, adopt, exit detection, status line rendering. Composes the existing `Tracker` and new tmux functions.

**Files:**
- Create: `internal/tabmgr/tabmgr.go`
- Create: `internal/tabmgr/adopt.go`
- Create: `internal/tabmgr/statusline.go`
- Test: `internal/tabmgr/tabmgr_test.go`

**Contracts:**

```go
// tabmgr.go — core tab manager

type Tab struct {
    WindowID    string
    SessionID   string  // matched from tracker, may be empty initially
    ProjectDir  string
    ProjectName string
    DisplayName string  // from state.Store naming
    Attention   string  // from capture.DeriveStatus: "waiting", "permission", "error", ""
    OnDone      string  // callback command (from ccs launch --on-done)
}

type Manager struct { ... }

// New creates a tab manager for the given tmux session.
func New(sessionName string, tracker *session.Tracker, state *state.Store, claudeFlags []string) *Manager

// Tabs returns the current list of managed tabs (excludes dashboard window).
func (m *Manager) Tabs() []Tab

// Launch creates a new tmux window with claude, registers it as a tab.
// Sets @ccs-managed window option and pane-exited hook on the new window.
// The pane-exited hook runs `ccs notify-exit --window <windowID>`.
// Returns the window ID.
func (m *Manager) Launch(projectDir string, resumeID string, prompt string, onDone string) (string, error)

// SwitchTo focuses the given tab's window.
func (m *Manager) SwitchTo(windowID string) error

// SwitchToDashboard focuses window 0 (dashboard).
func (m *Manager) SwitchToDashboard() error

// NextTab switches to the next tab (wraps around). Includes dashboard as tab 0.
func (m *Manager) NextTab() error

// PrevTab switches to the previous tab (wraps around). Includes dashboard as tab 0.
func (m *Manager) PrevTab() error

// UpdateAttention sets the attention state for a tab (called by capture tick).
func (m *Manager) UpdateAttention(windowID, attention string)

// HandleExit is called when a session window's process exits.
// Stores the pending on-done callback info. The actual callback fires after
// the transition summary is ready (see FirePendingCallback).
// Removes the tab from managed list.
func (m *Manager) HandleExit(windowID string)

// FirePendingCallback fires the on-done callback for a session, if one is pending.
// Called by the TUI model when TransitionSummaryMsg arrives for a session that
// had an on-done callback. Passes CCS_SESSION_ID, CCS_SESSION_PROJECT,
// CCS_SESSION_SUMMARY as environment variables.
// Returns true if a callback was fired.
func (m *Manager) FirePendingCallback(sessionID string, summary string) bool

// PendingCallbackSessionIDs returns session IDs that have pending on-done callbacks
// waiting for a summary. The TUI model checks this to know when to fire callbacks.
func (m *Manager) PendingCallbackSessionIDs() []string

// SyncFromTracker updates Tab.SessionID for tabs whose sessions have been
// matched by the tracker. Called on each refresh tick.
func (m *Manager) SyncFromTracker()

// CurrentWindowID returns the currently focused window ID.
func (m *Manager) CurrentWindowID() (string, error)
```

```go
// adopt.go — adoption of external Claude sessions

// ScanAndAdopt finds Claude processes in tmux windows outside the ccs session
// and moves them into the managed session via tmux.MoveWindow.
// Sets @ccs-managed and pane-exited hook on adopted windows.
// Uses tmux.AllPanesBySession() to find cross-session windows.
// Returns adopted window IDs.
func (m *Manager) ScanAndAdopt() ([]string, error)
```

```go
// statusline.go — tmux status line rendering

// RenderStatusLine updates the tmux status line with tab bar content.
// Line 1: tab names with attention indicators.
// Line 2: attention summary (sets status-lines to 1 if no attention needed, 2 if needed).
func (m *Manager) RenderStatusLine() error

// FormatTabBar builds the tmux format string for line 1 (tabs).
func FormatTabBar(tabs []Tab, currentWindowID string, maxWidth int) string

// FormatAttentionSummary builds the tmux format string for line 2.
// Returns empty string if no sessions need attention.
func FormatAttentionSummary(tabs []Tab) string
```

**Test Cases:**

```go
func TestFormatTabBar(t *testing.T) {
    // Basic: 3 tabs, second is current → "⌂ │ proj-a │ ▸ proj-b │ proj-c"
}

func TestFormatTabBarOverflow(t *testing.T) {
    // Many tabs exceeding maxWidth → visible tabs + "+N more"
}

func TestFormatTabBarAttention(t *testing.T) {
    // Tab with attention="waiting" → shows colored ● indicator
}

func TestFormatAttentionSummaryEmpty(t *testing.T) {
    // No attention needed → empty string (line 2 collapses)
}

func TestFormatAttentionSummary(t *testing.T) {
    // Two sessions need attention → "proj-a waiting for input · proj-b error in tests"
}

func TestLaunchCreatesTab(t *testing.T) {
    // After Launch(), Tabs() contains the new tab with correct fields
}

func TestNextPrevTabWraps(t *testing.T) {
    // NextTab from last tab wraps to dashboard, PrevTab from dashboard wraps to last
}

func TestHandleExitRemovesTab(t *testing.T) {
    // After HandleExit(), tab is no longer in Tabs()
}

func TestHandleExitStoresPendingCallback(t *testing.T) {
    // Tab with OnDone set → HandleExit stores pending, PendingCallbackSessionIDs includes it
}

func TestFirePendingCallback(t *testing.T) {
    // FirePendingCallback returns true and clears pending state
}

func TestSyncFromTrackerPopulatesSessionID(t *testing.T) {
    // After tracker matches a PID, SyncFromTracker fills in Tab.SessionID
}
```

**Constraints:**
- Tab manager does NOT own the tracker — receives it as a dependency
- Status line format uses tmux style tags: `#[fg=yellow]●#[default]` for colored attention indicators
- `@ccs-managed` window option set on every managed window (Launch and ScanAndAdopt)
- pane-exited hook set on every managed window: `ccs notify-exit --window <windowID>`
- The on-done callback fires as a shell command via `exec.Command("sh", "-c", onDone)` with env vars set
- On-done timeout: if TransitionSummaryMsg doesn't arrive within 10s of HandleExit, fire callback with empty summary. Use a goroutine with `time.After`.

**Verification:**
Run: `go test ./internal/tabmgr/ -v -count=1`
Expected: All tests pass

**Commit after passing.**

---

### Task 4: Startup, shutdown & keybinding lifecycle [Mode: Delegated]

Refactor main.go to implement the new startup/shutdown flow. Set up IPC server, status line, keybindings, initial adoption scan. Handle cleanup on normal and signal-based exit.

**Files:**
- Modify: `main.go`
- Create: `internal/tmux/keybind.go` (keybinding capture/restore logic)
- Test: `internal/tmux/keybind_test.go`

**Contracts:**

```go
// keybind.go — keybinding lifecycle

type SavedBindings struct {
    Space string // original command for prefix+Space
    One   string // original command for prefix+1
    Two   string // original command for prefix+2
}

// CaptureBindings reads the current prefix bindings for Space, 1, 2
// by parsing output of `tmux list-keys -T prefix`.
func CaptureBindings() (SavedBindings, error)

// InstallCCSBindings registers ccs-scoped keybindings with if-shell fallbacks.
// Uses `tmux show -wv @ccs-managed 2>/dev/null` for scoping.
// CCS actions: Space → select-window -t :0, 1 → previous-window, 2 → next-window.
// Fallback actions: the captured original bindings.
func InstallCCSBindings(saved SavedBindings) error

// RestoreBindings removes ccs bindings and restores originals.
func RestoreBindings(saved SavedBindings) error
```

```go
// main.go new startup flow (pseudocode):
//
// 1.  Parse CLI: "launch" → ipc.Launch() → exit
//                "notify-exit" → ipc.NotifyExit() → exit
//                else → continue to TUI
// 2.  config.Load()
// 3.  tmux bootstrap (existing)
// 4.  SetWindowOption(currentWindow, "@ccs-managed", "1") — mark dashboard window
// 5.  ipc.NewServer(socketPath) — start listening
// 6.  Signal handler: SIGINT, SIGTERM → cleanup()
// 7.  CaptureBindings() → saved
// 8.  InstallCCSBindings(saved)
// 9.  SetStatusLines(2)
// 10. tabmgr.New(...) → manager
// 11. manager.ScanAndAdopt() — adopt any existing Claude windows
// 12. Set ipc server handlers:
//       OnLaunch → manager.Launch(...)
//       OnExit → send TabExitMsg to bubbletea program via p.Send()
// 13. Start IPC server goroutine
// 14. Load sessions, state, watcher (existing)
// 15. tui.New(..., manager) — pass manager to TUI
// 16. tea.Run()
// 17. cleanup(): RestoreBindings, UnsetStatusFormat, SetStatusLines(1),
//     clear @ccs-managed from all windows, server.Close()
```

**Test Cases:**

```go
func TestCaptureBindingsParsesPrefixTable(t *testing.T) {
    // Given known `tmux list-keys -T prefix` output, correctly extracts Space/1/2 commands
}

func TestInstallCCSBindingsFormat(t *testing.T) {
    // Verify the if-shell command format is correct:
    // bind-key -T prefix Space if-shell "tmux show -wv @ccs-managed 2>/dev/null" "select-window -t :0" "<fallback>"
}

func TestRestoreBindingsResetsOriginals(t *testing.T) {
    // After RestoreBindings, the bindings match the original saved values
}
```

**Constraints:**
- Cleanup MUST run on both normal exit and signal (SIGINT/SIGTERM). Use `defer` + signal handler.
- If cleanup fails partially (e.g., can't restore bindings), log the error but continue cleanup.
- The `ccs launch` and `ccs notify-exit` subcommand paths exit before any tmux/TUI setup.
- Shutdown clears `@ccs-managed` from all windows in the session (prevents stale options after exit).
- IPC server's OnExit handler uses `p.Send(TabExitMsg{WindowID: notif.WindowID})` to inject into bubbletea's message loop (requires `*tea.Program` reference).

**Verification:**
Run: `go test ./internal/tmux/ -v -count=1 && go build -o /dev/null .`
Expected: All tests pass, build succeeds

**Commit after passing.**

---

### Task 5: TUI model refactor — dashboard simplification [Mode: Delegated]

Simplify the bubbletea TUI to work as the dashboard tab within the tabbed system. Remove follow mode, wire tab switching through the tab manager, add status line update tick.

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/launch.go`
- Modify: `internal/tui/views.go`
- Modify: `internal/tui/styles.go` (attention state colors)
- Test: `internal/tui/model_test.go`

**Changes to Model struct:**
- Add `tabmgr *tabmgr.Manager` field
- Remove `followID string` (follow mode gone)
- Remove pane capture fields only used for follow mode rendering (paneContent map stays — still used for status summaries)

**Changes to handleKey:**
- `enter` on active session → `m.tabmgr.SwitchTo(windowID)` (not TmuxLaunchResume)
- `enter` on inactive session → `m.tabmgr.Launch(projectDir, sessionID, "", "")` then switch
- `enter` on project dir in search → `m.tabmgr.Launch(dirPath, "", "", "")` then switch
- Remove `f` key handler (follow mode)
- Remove `1-9` number shortcuts (tmux handles those)
- Keep all other bindings unchanged

**New message types and ticks:**
```go
// TabExitMsg signals that a session tab's process exited (from IPC notify-exit).
type TabExitMsg struct {
    WindowID string
}

// StatusLineTickMsg triggers periodic status line updates.
type StatusLineTickMsg struct{}
```

**New handler: StatusLineTickMsg**
- Calls `m.tabmgr.RenderStatusLine()` to update the tmux status line
- Polls `m.tabmgr.CurrentWindowID()` to keep tab bar highlight current
- Fires every 1s (same as pane capture tick)
- Scheduled in `Init()` alongside other ticks

**New handler: TabExitMsg**
- Calls `m.tabmgr.HandleExit(msg.WindowID)`
- Triggers a refresh to update session state
- Checks `m.tabmgr.PendingCallbackSessionIDs()` — if the exited session has a pending callback, starts a 10s timer goroutine

**Modified handler: TransitionSummaryMsg**
- After storing the summary, checks `m.tabmgr.FirePendingCallback(sessionID, summary)` — if a callback was pending, it fires here with the summary

**Changes to views.go:**
- Active section: simple rows with attention badges (colored by state), no expanded status lines
- Attention state colors: yellow (waiting), orange (permission), red (error), green (thinking/working)
- Remove follow mode rendering (followView function)
- Detail pane unchanged (summary + conversation text)
- Update help screen (`?` key) to reflect removed keybindings (f, 1-9) and new tmux-level bindings

**Changes to launch.go:**
- `TmuxLaunchResume` and `TmuxLaunchNew` removed — replaced by `tabmgr.Launch`
- `TmuxSwitch` removed — replaced by `tabmgr.SwitchTo`
- Messages `TmuxLaunchDoneMsg` and `TmuxSwitchDoneMsg` removed — tab manager handles directly

**Test Cases:**

```go
func TestEnterOnActiveSessionSwitches(t *testing.T) {
    // enter on active session calls SwitchTo, not LaunchResume
}

func TestEnterOnInactiveSessionLaunches(t *testing.T) {
    // enter on inactive session calls Launch (creates new tab)
}

func TestFollowKeyRemoved(t *testing.T) {
    // 'f' key in main mode does nothing
}

func TestNumberKeysRemoved(t *testing.T) {
    // '1'-'9' keys in main mode do nothing
}

func TestAttentionBadgeColors(t *testing.T) {
    // "waiting" → yellow style, "error" → red style, "permission" → orange style
}

func TestTabExitMsgTriggersHandleExit(t *testing.T) {
    // TabExitMsg calls tabmgr.HandleExit and triggers refresh
}

func TestStatusLineTickScheduled(t *testing.T) {
    // Init() includes StatusLineTickMsg in batch commands
}
```

**Constraints:**
- The model must accept a `*tabmgr.Manager` in `tui.New()` — update the constructor signature
- Tab manager handles the actual tmux commands; model triggers them via methods
- Status summaries (haiku) continue working — pane capture tick and naming still active
- `paneContent` map still populated by capture ticks — used for AI summaries
- Status line tick and pane capture tick can share the same interval (1s)

**Verification:**
Run: `go test ./internal/tui/ -v -count=1 && go build -o ~/.local/bin/ccs .`
Expected: All tests pass, binary installs

**Commit after passing.**

---

## Execution
**Skill:** superpowers:subagent-driven-development
- Task 1 [Mode: Direct]: Opus implements directly — thin tmux CLI wrappers
- Task 2 [Mode: Delegated]: IPC package + CLI subcommand + exit notification
- Task 3 [Mode: Delegated]: Tab manager + status line + on-done callbacks
- Task 4 [Mode: Delegated]: Startup/shutdown lifecycle + keybindings
- Task 5 [Mode: Delegated]: TUI model refactor + status line tick
