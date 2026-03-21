package tabmgr

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"ccs/internal/session"
	"ccs/internal/state"
	"ccs/internal/tmux"
)

// Tab represents a managed tmux window running a Claude session.
type Tab struct {
	WindowID    string
	SessionID   string // matched from tracker, may be empty initially
	ProjectDir  string
	ProjectName string
	DisplayName string // from state.Store naming
	Attention   string // "waiting", "permission", "error", ""
	OnDone      string // callback command
}

// pendingCallback stores info for on-done callbacks awaiting a summary.
type pendingCallback struct {
	SessionID  string
	ProjectDir string
	OnDone     string
}

// Manager manages tmux tabs for Claude sessions.
type Manager struct {
	sessionName string
	tracker     *session.Tracker
	state       *state.Store
	claudeFlags []string
	tabs        []Tab
	pending     map[string]pendingCallback // sessionID -> pending callback
	mu          sync.Mutex
}

// New creates a new tab Manager.
func New(sessionName string, tracker *session.Tracker, state *state.Store, claudeFlags []string) *Manager {
	return &Manager{
		sessionName: sessionName,
		tracker:     tracker,
		state:       state,
		claudeFlags: claudeFlags,
		pending:     make(map[string]pendingCallback),
	}
}

// Tabs returns the current list of managed tabs.
func (m *Manager) Tabs() []Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Tab, len(m.tabs))
	copy(result, m.tabs)
	return result
}

// Launch creates a new tmux window with claude, registers it as a tab.
// Sets @ccs-managed window option and pane-exited hook.
// The hook runs: ccs notify-exit --window <windowID>
// For resumeID != "": claude --resume <id> [flags]
// For prompt != "": claude [flags] <prompt>
// For new session: claude [flags]
// Returns the window ID.
func (m *Manager) Launch(projectDir string, resumeID string, prompt string, onDone string) (string, error) {
	projectName := filepath.Base(projectDir)

	// Build command args
	cmd := []string{"claude"}
	cmd = append(cmd, m.claudeFlags...)
	if resumeID != "" {
		cmd = append(cmd, "--resume", resumeID)
	} else if prompt != "" {
		cmd = append(cmd, prompt)
	}

	// Build window name
	windowName := projectName
	if len([]rune(windowName)) > 30 {
		windowName = string([]rune(windowName)[:30])
	}

	windowID, err := tmux.NewWindow(windowName, projectDir, cmd)
	if err != nil {
		return "", fmt.Errorf("creating tmux window: %w", err)
	}

	// Set @ccs-managed option
	if err := tmux.SetWindowOption(windowID, "ccs-managed", "1"); err != nil {
		return windowID, fmt.Errorf("setting @ccs-managed: %w", err)
	}

	// Set pane-exited hook
	hookCmd := fmt.Sprintf("run-shell 'ccs notify-exit --window %s'", windowID)
	if err := tmux.SetHook(windowID, "pane-exited", hookCmd); err != nil {
		return windowID, fmt.Errorf("setting pane-exited hook: %w", err)
	}

	tab := Tab{
		WindowID:    windowID,
		SessionID:   resumeID, // known if resuming, empty for new
		ProjectDir:  projectDir,
		ProjectName: projectName,
		OnDone:      onDone,
	}

	// If resuming, get display name from state
	if resumeID != "" {
		if st, ok := m.state.Get(resumeID); ok && st.Name != "" {
			tab.DisplayName = st.Name
		}
	}

	m.mu.Lock()
	m.tabs = append(m.tabs, tab)
	m.mu.Unlock()

	return windowID, nil
}

// SwitchTo focuses the given tab's window.
func (m *Manager) SwitchTo(windowID string) error {
	return tmux.SelectWindow(windowID)
}

// SwitchToDashboard focuses window 0 (the dashboard).
func (m *Manager) SwitchToDashboard() error {
	// Window index 0 in the ccs session
	return tmux.SelectWindow(m.sessionName + ":0")
}

// NextTab switches to the next tab (wraps). Dashboard (index -1 conceptually) is included.
func (m *Manager) NextTab() error {
	currentID, err := tmux.CurrentWindowID()
	if err != nil {
		return err
	}

	m.mu.Lock()
	tabs := make([]Tab, len(m.tabs))
	copy(tabs, m.tabs)
	m.mu.Unlock()

	// Build ordered list: dashboard + tabs
	windowIDs := make([]string, 0, len(tabs)+1)
	dashboardID := m.sessionName + ":0"
	windowIDs = append(windowIDs, dashboardID)
	for _, t := range tabs {
		windowIDs = append(windowIDs, t.WindowID)
	}

	// Find current and switch to next
	for i, wid := range windowIDs {
		if wid == currentID {
			nextIdx := (i + 1) % len(windowIDs)
			return tmux.SelectWindow(windowIDs[nextIdx])
		}
	}

	// Current window not in our list — go to dashboard
	return tmux.SelectWindow(dashboardID)
}

// PrevTab switches to the previous tab (wraps).
func (m *Manager) PrevTab() error {
	currentID, err := tmux.CurrentWindowID()
	if err != nil {
		return err
	}

	m.mu.Lock()
	tabs := make([]Tab, len(m.tabs))
	copy(tabs, m.tabs)
	m.mu.Unlock()

	// Build ordered list: dashboard + tabs
	windowIDs := make([]string, 0, len(tabs)+1)
	dashboardID := m.sessionName + ":0"
	windowIDs = append(windowIDs, dashboardID)
	for _, t := range tabs {
		windowIDs = append(windowIDs, t.WindowID)
	}

	// Find current and switch to previous
	for i, wid := range windowIDs {
		if wid == currentID {
			prevIdx := (i - 1 + len(windowIDs)) % len(windowIDs)
			return tmux.SelectWindow(windowIDs[prevIdx])
		}
	}

	// Current window not in our list — go to dashboard
	return tmux.SelectWindow(dashboardID)
}

// UpdateAttention sets the attention state for a tab.
func (m *Manager) UpdateAttention(windowID, attention string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.tabs {
		if m.tabs[i].WindowID == windowID {
			m.tabs[i].Attention = attention
			return
		}
	}
}

// HandleExit is called when a session window's process exits.
// If the tab has an OnDone callback, stores it as pending (waiting for summary).
// Starts a 10s timeout goroutine — if no summary arrives, fires callback with empty summary.
// Removes the tab from managed list.
func (m *Manager) HandleExit(windowID string) {
	m.mu.Lock()

	var removed Tab
	found := false
	for i, t := range m.tabs {
		if t.WindowID == windowID {
			removed = t
			found = true
			m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
			break
		}
	}

	if !found {
		m.mu.Unlock()
		return
	}

	if removed.OnDone != "" && removed.SessionID != "" {
		m.pending[removed.SessionID] = pendingCallback{
			SessionID:  removed.SessionID,
			ProjectDir: removed.ProjectDir,
			OnDone:     removed.OnDone,
		}
		sessionID := removed.SessionID
		m.mu.Unlock()

		// 10s timeout: fire with empty summary if no TransitionSummaryMsg arrives
		go func() {
			time.Sleep(10 * time.Second)
			m.mu.Lock()
			if pc, ok := m.pending[sessionID]; ok {
				delete(m.pending, sessionID)
				m.mu.Unlock()
				fireCallback(pc, "")
			} else {
				m.mu.Unlock()
			}
		}()
	} else {
		m.mu.Unlock()
	}
}

// FirePendingCallback fires the on-done callback for a session if pending.
// Called when TransitionSummaryMsg arrives. Passes env vars:
// CCS_SESSION_ID, CCS_SESSION_PROJECT, CCS_SESSION_SUMMARY
// Returns true if a callback was fired.
func (m *Manager) FirePendingCallback(sessionID string, summary string) bool {
	m.mu.Lock()
	pc, ok := m.pending[sessionID]
	if ok {
		delete(m.pending, sessionID)
	}
	m.mu.Unlock()

	if !ok {
		return false
	}

	fireCallback(pc, summary)
	return true
}

// PendingCallbackSessionIDs returns session IDs with pending callbacks.
func (m *Manager) PendingCallbackSessionIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids := make([]string, 0, len(m.pending))
	for id := range m.pending {
		ids = append(ids, id)
	}
	return ids
}

// SyncFromTracker updates Tab.SessionID from tracker's matched sessions.
func (m *Manager) SyncFromTracker() {
	tmuxWindows := m.tracker.TmuxWindowIDs()

	// Build reverse map: windowID -> sessionID
	windowToSession := make(map[string]string)
	for sessID, winID := range tmuxWindows {
		windowToSession[winID] = sessID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.tabs {
		if m.tabs[i].SessionID == "" {
			if sessID, ok := windowToSession[m.tabs[i].WindowID]; ok {
				m.tabs[i].SessionID = sessID
			}
		}

		// Update display name from state
		if m.tabs[i].SessionID != "" {
			if st, ok := m.state.Get(m.tabs[i].SessionID); ok && st.Name != "" {
				m.tabs[i].DisplayName = st.Name
			}
		}
	}
}

// CurrentWindowID returns the currently focused window ID.
func (m *Manager) CurrentWindowID() (string, error) {
	return tmux.CurrentWindowID()
}

// fireCallback executes the on-done callback command with env vars set.
func fireCallback(pc pendingCallback, summary string) {
	cmd := exec.Command("sh", "-c", pc.OnDone)
	cmd.Env = append(cmd.Environ(),
		"CCS_SESSION_ID="+pc.SessionID,
		"CCS_SESSION_PROJECT="+pc.ProjectDir,
		"CCS_SESSION_SUMMARY="+summary,
	)
	_ = cmd.Run()
}
