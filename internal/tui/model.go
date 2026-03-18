package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"

	"ccs/internal/activity"
	"ccs/internal/capture"
	"ccs/internal/config"
	"ccs/internal/naming"
	"ccs/internal/project"
	"ccs/internal/session"
	"ccs/internal/state"
	"ccs/internal/types"
	"ccs/internal/watcher"
)

// Messages for launching sessions.
type RefreshMsg struct{}
type ActivityUpdateMsg struct {
	SessionID string
	Entries   []activity.Entry
}
type TickMsg struct{}

// PaneCaptureMsg delivers a pane capture result.
type PaneCaptureMsg struct {
	Snapshot capture.PaneSnapshot
	Err      error
}

// PaneCaptureTickMsg triggers periodic pane capture polling.
type PaneCaptureTickMsg struct{}

// AutoNameMsg delivers the result of an auto-naming attempt.
type AutoNameMsg struct {
	SessionID string
	Name      string
	Err       error
}

// AutoNameTriggerMsg triggers naming for a session after a delay.
type AutoNameTriggerMsg struct {
	SessionID string
}

// SearchResult is a union type for search results: either a session or a project directory.
type SearchResult struct {
	Session *types.Session // nil for project dir results
	DirPath string         // for project dirs
	DirName string         // display name
}

type Model struct {
	sessions          []types.Session
	filtered          []types.Session // active + open (+ done/untracked when visible)
	config            *types.Config
	filter            textinput.Model
	sessionIdx        int
	filtering         bool
	showDoneUntracked bool
	showHelp          bool
	showPrefs         bool
	prefsIdx          int
	confirming        bool
	confirmSess       *types.Session
	renaming          bool
	renameInput       textinput.Model
	renameTarget      string // session ID being renamed
	width             int
	height            int
	errMsg            string
	pendingG          bool // true when 'g' was pressed, waiting for second 'g'
	sortField         types.SortField
	sortDir           types.SortDir
	tracker           *session.Tracker
	state             *state.Store
	watcher           *watcher.Watcher
	activities        map[string][]activity.Entry    // sessionID -> recent entries
	followID          string                         // session ID being followed (empty = normal)
	paneContent       map[string]capture.PaneSnapshot // sessionID -> latest snapshot
	projectsDir       string                         // ~/.claude/projects path
	projectsRoot      string                         // ~/Projects/ path
	projectDirs       []project.ProjectDir           // scanned project dirs
	searchResults     []SearchResult                 // populated when filtering
	searchIdx         int                            // index into searchResults
	prevActiveIDs     map[string]bool                // previous refresh active set (for transition detection)
}

func New(sessions []types.Session, cfg *types.Config, tracker *session.Tracker, st *state.Store, w *watcher.Watcher, projectsDir string, projectsRoot string) Model {
	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.Prompt = "/ "
	ti.CharLimit = 64

	m := Model{
		sessions:      sessions,
		config:        cfg,
		filter:        ti,
		sortField:     types.SortByTime,
		sortDir:       types.SortDesc,
		tracker:       tracker,
		state:         st,
		watcher:       w,
		activities:    make(map[string][]activity.Entry),
		paneContent:   make(map[string]capture.PaneSnapshot),
		projectsDir:   projectsDir,
		projectsRoot:  projectsRoot,
		projectDirs:   project.ScanProjectDirs(projectsRoot),
		prevActiveIDs: make(map[string]bool),
	}
	_ = computeStateStatuses(m.sessions, m.tracker, m.state)
	m.applyFilter()

	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd(), paneCaptureTickCmd()}

	if m.watcher != nil {
		go m.watcher.Run()
		cmds = append(cmds, watchCmd(m.watcher))

		// Watch all currently-active sessions
		for _, s := range m.sessions {
			if s.IsActive && s.FilePath != "" {
				_ = m.watcher.Watch(s.ID, s.FilePath)
			}
		}
	}

	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case TmuxLaunchDoneMsg:
		if msg.Err != nil {
			m.errMsg = fmt.Sprintf("Tmux launch error: %v", msg.Err)
		}
		return m, refreshCmd()

	case TmuxSwitchDoneMsg:
		if msg.Err != nil {
			m.errMsg = fmt.Sprintf("Tmux switch error: %v", msg.Err)
		}
		return m, nil

	case ActivityUpdateMsg:
		m.activities[msg.SessionID] = msg.Entries
		if m.watcher != nil {
			return m, watchCmd(m.watcher)
		}
		return m, nil

	case PaneCaptureMsg:
		if msg.Err == nil {
			m.paneContent[msg.Snapshot.SessionID] = msg.Snapshot
		}
		return m, nil

	case PaneCaptureTickMsg:
		var cmds []tea.Cmd
		captured := make(map[string]bool)
		tmuxWindows := m.tracker.TmuxWindowIDs()
		for sessID := range tmuxWindows {
			if cmd := m.captureCmdForSession(sessID); cmd != nil {
				cmds = append(cmds, cmd)
				captured[sessID] = true
			}
		}
		if m.followID != "" && !captured[m.followID] {
			if cmd := m.captureCmdForSession(m.followID); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		cmds = append(cmds, paneCaptureTickCmd())
		return m, tea.Batch(cmds...)

	case AutoNameMsg:
		if msg.Name != "" {
			m.state.SetName(msg.SessionID, msg.Name, "auto")
		}
		return m, nil

	case AutoNameTriggerMsg:
		content := m.namingContent(msg.SessionID)
		if content != "" {
			return m, autoNameCmd(msg.SessionID, content, m.config.AutoNameLines)
		}
		return m, nil

	case TickMsg:
		cmd := m.handleRefresh()
		return m, tea.Batch(cmd, tickCmd())

	case RefreshMsg:
		return m, m.handleRefresh()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Pass messages to text input when filtering
	if m.filtering {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.applyFilter()
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		return m, tea.Quit
	}

	m.errMsg = ""

	// When filtering, most keys go to the text input
	if m.filtering {
		switch key {
		case "esc":
			m.filtering = false
			m.filter.Blur()
			m.filter.SetValue("")
			m.applyFilter()
			return m, nil
		case "enter":
			if len(m.searchResults) > 0 && m.searchIdx < len(m.searchResults) {
				r := m.searchResults[m.searchIdx]
				m.filtering = false
				m.filter.Blur()
				m.filter.SetValue("")
				m.applyFilter()
				if r.Session != nil {
					// Session → switch or resume
					tmuxWindows := m.tracker.TmuxWindowIDs()
					if wid, ok := tmuxWindows[r.Session.ID]; ok {
						return m, TmuxSwitch(wid)
					}
					return m, TmuxLaunchResume(*r.Session, m.config.ClaudeFlags, m.tracker)
				}
				// Project dir → launch new session
				return m, TmuxLaunchNew(r.DirPath, r.DirName, m.config.ClaudeFlags, m.tracker)
			}
			m.filtering = false
			m.filter.Blur()
			return m, nil
		case "up":
			if m.searchIdx > 0 {
				m.searchIdx--
			}
			return m, nil
		case "down":
			if m.searchIdx < len(m.searchResults)-1 {
				m.searchIdx++
			}
			return m, nil
		case "c":
			// Complete in search mode
			if m.searchIdx < len(m.searchResults) {
				r := m.searchResults[m.searchIdx]
				if r.Session != nil {
					if r.Session.StateStatus == types.StatusActive {
						m.errMsg = "Session still running — complete after it ends"
					} else if r.Session.StateStatus != types.StatusDone {
						m.state.MarkDone(r.Session.ID)
						m.applyFilter()
					}
				}
			}
			return m, nil
		case "o":
			// Reopen in search mode
			if m.searchIdx < len(m.searchResults) {
				r := m.searchResults[m.searchIdx]
				if r.Session != nil && r.Session.StateStatus == types.StatusDone {
					m.state.Reopen(r.Session.ID)
					m.applyFilter()
				}
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			m.applyFilter()
			return m, cmd
		}
	}

	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	if m.showPrefs {
		const prefsCount = 3
		switch key {
		case "j", "down":
			if m.prefsIdx < prefsCount-1 {
				m.prefsIdx++
			}
		case "k", "up":
			if m.prefsIdx > 0 {
				m.prefsIdx--
			}
		case "enter", " ":
			switch m.prefsIdx {
			case 0:
				m.config.RelativeNumbers = !m.config.RelativeNumbers
				if err := config.Save(m.config); err != nil {
					m.errMsg = fmt.Sprintf("Config save error: %v", err)
				}
			case 1:
				m.config.ActivityLines = cycleValue(m.config.ActivityLines, []int{3, 5, 10, 15})
				if err := config.Save(m.config); err != nil {
					m.errMsg = fmt.Sprintf("Config save error: %v", err)
				}
			case 2:
				m.config.AutoNameLines = cycleValue(m.config.AutoNameLines, []int{10, 20, 30, 50})
				if err := config.Save(m.config); err != nil {
					m.errMsg = fmt.Sprintf("Config save error: %v", err)
				}
			}
		case "esc", "p", "q":
			m.showPrefs = false
		}
		return m, nil
	}

	if m.confirming {
		switch key {
		case "y":
			if m.confirmSess != nil {
				if err := os.Remove(m.confirmSess.FilePath); err != nil && !os.IsNotExist(err) {
					m.errMsg = fmt.Sprintf("Delete error: %v", err)
				}
				subagentsDir := strings.TrimSuffix(m.confirmSess.FilePath, ".jsonl")
				if err := os.RemoveAll(subagentsDir); err != nil {
					m.errMsg = fmt.Sprintf("Delete error: %v", err)
				}
				m.state.Remove(m.confirmSess.ID)
				m.confirming = false
				m.confirmSess = nil
				return m, refreshCmd()
			}
		case "n", "esc":
			m.confirming = false
			m.confirmSess = nil
		}
		return m, nil
	}

	// Rename mode
	if m.renaming {
		switch key {
		case "enter":
			value := strings.TrimSpace(m.renameInput.Value())
			if value != "" {
				m.state.SetName(m.renameTarget, value, "manual")
			}
			m.renaming = false
			m.renameTarget = ""
			return m, nil
		case "esc":
			m.renaming = false
			m.renameTarget = ""
			return m, nil
		default:
			var cmd tea.Cmd
			m.renameInput, cmd = m.renameInput.Update(msg)
			return m, cmd
		}
	}

	if m.pendingG {
		m.pendingG = false
		if key == "g" {
			m.sessionIdx = 0
			return m, nil
		}
	}

	// Number shortcuts: 1-9 for the Nth visible session (active + open combined)
	if key >= "1" && key <= "9" {
		n := int(key[0] - '0')
		idx := n - 1
		if idx < len(m.filtered) {
			sess := m.filtered[idx]
			tmuxWindows := m.tracker.TmuxWindowIDs()
			if wid, ok := tmuxWindows[sess.ID]; ok {
				return m, TmuxSwitch(wid)
			}
			return m, TmuxLaunchResume(sess, m.config.ClaudeFlags, m.tracker)
		}
		return m, nil
	}

	switch key {
	case "q":
		return m, tea.Quit

	case "/":
		m.filtering = true
		m.filter.Focus()
		return m, textinput.Blink

	case "up", "k":
		return m.handleNavigation("up")

	case "down", "j":
		return m.handleNavigation("down")

	case "G":
		if len(m.filtered) > 0 {
			m.sessionIdx = len(m.filtered) - 1
		}
		return m, nil

	case "g":
		m.pendingG = true
		return m, nil

	case "enter":
		return m.handleEnter()

	case "c":
		if len(m.filtered) > 0 {
			sess := m.filtered[m.sessionIdx]
			if sess.StateStatus == types.StatusActive {
				m.errMsg = "Session still running — complete after it ends"
			} else if sess.StateStatus != types.StatusDone {
				m.state.MarkDone(sess.ID)
				m.applyFilter()
			}
		}
		return m, nil

	case "o":
		if len(m.filtered) > 0 {
			sess := m.filtered[m.sessionIdx]
			if sess.StateStatus == types.StatusDone {
				m.state.Reopen(sess.ID)
				m.applyFilter()
			}
		}
		return m, nil

	case "R":
		if len(m.filtered) > 0 {
			sess := m.filtered[m.sessionIdx]
			ri := textinput.New()
			ri.Placeholder = "session name..."
			ri.Prompt = "Rename: "
			ri.CharLimit = 64
			ri.SetValue(m.displayName(sess))
			ri.Focus()
			m.renameInput = ri
			m.renameTarget = sess.ID
			m.renaming = true
			return m, textinput.Blink
		}
		return m, nil

	case "d":
		if len(m.filtered) > 0 {
			sess := m.filtered[m.sessionIdx]
			m.confirming = true
			m.confirmSess = &sess
		}
		return m, nil

	case "s":
		m.sortField = m.sortField.Next()
		m.sessionIdx = 0
		m.applyFilter()
		return m, nil

	case "r":
		m.sortDir = m.sortDir.Toggle()
		m.sessionIdx = 0
		m.applyFilter()
		return m, nil

	case "h":
		m.showDoneUntracked = !m.showDoneUntracked
		m.applyFilter()
		return m, nil

	case "?":
		m.showHelp = !m.showHelp
		return m, nil

	case "p":
		m.showPrefs = !m.showPrefs
		m.prefsIdx = 0
		return m, nil

	case "f":
		return m.handleFollow()

	case "N":
		if len(m.filtered) > 0 {
			sess := m.filtered[m.sessionIdx]
			content := m.namingContent(sess.ID)
			if content != "" {
				return m, autoNameCmd(sess.ID, content, m.config.AutoNameLines)
			}
		}
		return m, nil

	case "esc":
		if m.followID != "" {
			m.followID = ""
			return m, nil
		}
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.applyFilter()
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleNavigation(key string) (Model, tea.Cmd) {
	switch key {
	case "up":
		if m.sessionIdx > 0 {
			m.sessionIdx--
		}
	case "down":
		if m.sessionIdx < len(m.filtered)-1 {
			m.sessionIdx++
		}
	}
	return m, nil
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	sess := m.filtered[m.sessionIdx]
	// Active sessions with tmux windows → switch
	tmuxWindows := m.tracker.TmuxWindowIDs()
	if wid, ok := tmuxWindows[sess.ID]; ok {
		return m, TmuxSwitch(wid)
	}
	// All others → resume in new tmux window
	return m, TmuxLaunchResume(sess, m.config.ClaudeFlags, m.tracker)
}

func (m Model) handleFollow() (Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	sess := m.filtered[m.sessionIdx]

	if m.followID == sess.ID {
		m.followID = ""
		return m, nil
	}

	if sess.ActiveSource != types.SourceTmux {
		m.errMsg = "Can only follow sessions with tmux windows"
		return m, nil
	}

	m.followID = sess.ID
	return m, m.captureCmdForSession(sess.ID)
}

func (m *Model) applyFilter() {
	var selectedID string
	if m.sessionIdx < len(m.filtered) {
		selectedID = m.filtered[m.sessionIdx].ID
	}

	query := m.filter.Value()

	hiddenSet := make(map[string]bool, len(m.config.HiddenSessions))
	for _, id := range m.config.HiddenSessions {
		hiddenSet[id] = true
	}

	// Filter out hidden sessions
	var source []types.Session
	for _, s := range m.sessions {
		if !hiddenSet[s.ID] {
			source = append(source, s)
		}
	}

	if query != "" {
		// Fuzzy search all sessions + project dirs → searchResults
		m.searchResults = nil

		// Match sessions
		sessTargets := make([]string, len(source))
		for i, s := range source {
			sessTargets[i] = s.ProjectName + " " + m.displayName(s) + " " + s.Title
		}
		sessMatches := fuzzy.Find(query, sessTargets)
		for _, match := range sessMatches {
			s := source[match.Index]
			m.searchResults = append(m.searchResults, SearchResult{Session: &s})
		}

		// Match project dirs
		dirTargets := make([]string, len(m.projectDirs))
		for i, d := range m.projectDirs {
			dirTargets[i] = d.Name
		}
		dirMatches := fuzzy.Find(query, dirTargets)
		for _, match := range dirMatches {
			d := m.projectDirs[match.Index]
			m.searchResults = append(m.searchResults, SearchResult{DirPath: d.Path, DirName: d.Name})
		}

		// Also build filtered for compatibility (session results only)
		m.filtered = nil
		for _, r := range m.searchResults {
			if r.Session != nil {
				m.filtered = append(m.filtered, *r.Session)
			}
		}
		if m.searchIdx >= len(m.searchResults) {
			m.searchIdx = max(0, len(m.searchResults)-1)
		}
		m.restoreSelection(selectedID)
		return
	}

	m.searchResults = nil

	// Partition sessions by state: active first, then open, then done/untracked
	var active, open, rest []types.Session
	for _, s := range source {
		switch s.StateStatus {
		case types.StatusActive:
			active = append(active, s)
		case types.StatusOpen:
			open = append(open, s)
		default:
			rest = append(rest, s)
		}
	}

	// Active: always sorted by last active (most recent first)
	sort.SliceStable(active, func(i, j int) bool {
		return active[i].LastActive.After(active[j].LastActive)
	})

	// Open: sorted by user's sort choice
	m.sortSlice(open)

	// Done/untracked: sorted same as open
	m.sortSlice(rest)

	m.filtered = nil
	m.filtered = append(m.filtered, active...)
	m.filtered = append(m.filtered, open...)
	if m.showDoneUntracked {
		m.filtered = append(m.filtered, rest...)
	}

	m.restoreSelection(selectedID)
}

func (m *Model) sortSlice(sessions []types.Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		var less bool
		switch m.sortField {
		case types.SortByTime:
			less = sessions[i].LastActive.After(sessions[j].LastActive)
		case types.SortByContext:
			less = sessions[i].ContextPct > sessions[j].ContextPct
		case types.SortBySize:
			less = sessions[i].FileSize > sessions[j].FileSize
		case types.SortByName:
			less = strings.ToLower(sessions[i].Title) < strings.ToLower(sessions[j].Title)
		}
		if m.sortDir == types.SortAsc {
			less = !less
		}
		return less
	})
}

func (m *Model) restoreSelection(id string) {
	if id != "" {
		for i, s := range m.filtered {
			if s.ID == id {
				m.sessionIdx = i
				m.clampIndex()
				return
			}
		}
	}
	m.clampIndex()
}

func (m *Model) clampIndex() {
	if m.sessionIdx >= len(m.filtered) {
		m.sessionIdx = max(0, len(m.filtered)-1)
	}
}

// activeSessions returns the active sessions from the filtered list.
func (m *Model) activeSessions() []types.Session {
	var result []types.Session
	for _, s := range m.filtered {
		if s.StateStatus == types.StatusActive {
			result = append(result, s)
		}
	}
	return result
}

// openSessions returns the open sessions from the filtered list.
func (m *Model) openSessions() []types.Session {
	var result []types.Session
	for _, s := range m.filtered {
		if s.StateStatus == types.StatusOpen {
			result = append(result, s)
		}
	}
	return result
}

// doneCount returns the number of done sessions in the full session list.
func (m *Model) doneCount() int {
	count := 0
	for _, s := range m.sessions {
		if s.StateStatus == types.StatusDone {
			count++
		}
	}
	return count
}

// displayName returns the best available name for a session using the fallback chain:
// manual name > auto name > session name > title
func (m *Model) displayName(s types.Session) string {
	if ss, ok := m.state.Get(s.ID); ok && ss.Name != "" {
		return ss.Name
	}
	if s.SessionName != "" {
		return s.SessionName
	}
	return s.Title
}

// activeRowLines returns how many lines an active row takes.
func (m *Model) activeRowLines(s types.Session) int {
	lines := 1 // header line
	if snap, ok := m.paneContent[s.ID]; ok && snap.Content != "" {
		// Show 1-2 lines of pane capture
		paneLines := strings.Split(snap.Content, "\n")
		n := 2
		if len(paneLines) < n {
			n = len(paneLines)
		}
		lines += n
	}
	return lines
}

// scrollWindow returns the start and end indices for the visible open session window.
// Active sessions are always fully visible — this handles open session scrolling.
func (m *Model) scrollWindow() (int, int) {
	active := m.activeSessions()
	openList := m.openSessions()
	nActive := len(active)
	nOpen := len(openList)

	if nOpen == 0 {
		return 0, 0
	}

	// Calculate space used by active section
	activeLines := 0
	for _, s := range active {
		activeLines += m.activeRowLines(s)
	}
	if nActive > 0 {
		activeLines += 2 // ACTIVE header + blank
	}

	// Fixed overhead: outer border(2) + title(1) + open header(1) + footer(2)
	fixedOverhead := 6 + activeLines

	showDetail := m.sessionIdx >= nActive && nOpen > 0
	if showDetail {
		fixedOverhead += m.detailPaneLines()
	}

	availHeight := m.height - fixedOverhead
	maxRows := availHeight
	if maxRows < 3 {
		maxRows = 3
	}
	if maxRows > nOpen {
		maxRows = nOpen
	}

	// sessionIdx relative to open section
	openIdx := m.sessionIdx - nActive
	if openIdx < 0 {
		openIdx = 0
	}

	half := maxRows / 2
	start := openIdx - half
	if start < 0 {
		start = 0
	}
	if start > nOpen-maxRows {
		start = max(0, nOpen-maxRows)
	}
	end := start + maxRows
	if end > nOpen {
		end = nOpen
	}
	return start, end
}

func (m *Model) activityLines() int {
	if m.config.ActivityLines > 0 {
		return m.config.ActivityLines
	}
	return 5
}

// detailPaneLines returns a fixed height for the detail pane.
func (m *Model) detailPaneLines() int {
	if len(m.filtered) == 0 {
		return 0
	}
	return 7 + m.activityLines()
}

// computeStateStatuses sets StateStatus on each session by merging tracker and state store.
// Returns IDs of sessions that were just promoted to open.
func computeStateStatuses(sessions []types.Session, tracker *session.Tracker, st *state.Store) []string {
	openIDs := tracker.OpenSessionIDs()
	var promoted []string
	for i := range sessions {
		id := sessions[i].ID
		if openIDs[id] {
			sessions[i].StateStatus = types.StatusActive
			if !st.Has(id) {
				st.MarkOpen(id)
				promoted = append(promoted, id)
			}
		} else if ss, ok := st.Get(id); ok {
			switch ss.Status {
			case "open":
				sessions[i].StateStatus = types.StatusOpen
			case "done":
				sessions[i].StateStatus = types.StatusDone
			default:
				sessions[i].StateStatus = types.StatusUntracked
			}
		} else {
			sessions[i].StateStatus = types.StatusUntracked
		}
	}
	return promoted
}

func autoNameCmd(sessionID, content string, maxLines int) tea.Cmd {
	return func() tea.Msg {
		result := naming.GenerateName(sessionID, content, maxLines)
		return AutoNameMsg{SessionID: result.SessionID, Name: result.Name, Err: result.Err}
	}
}

func (m *Model) namingContent(sessionID string) string {
	if snap, ok := m.paneContent[sessionID]; ok && snap.Content != "" {
		return snap.Content
	}
	for _, s := range m.sessions {
		if s.ID == sessionID && s.FilePath != "" {
			return activity.TailFileLines(s.FilePath, m.config.AutoNameLines)
		}
	}
	return ""
}

func refreshCmd() tea.Cmd {
	return func() tea.Msg {
		return RefreshMsg{}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}

func watchCmd(w *watcher.Watcher) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-w.Updates()
		if !ok {
			return nil
		}
		return ActivityUpdateMsg{
			SessionID: update.SessionID,
			Entries:   update.Entries,
		}
	}
}

func (m *Model) handleRefresh() tea.Cmd {
	sessions, err := session.DiscoverSessions(m.projectsDir)
	if err != nil {
		m.errMsg = fmt.Sprintf("Session discovery error: %v", err)
		return nil
	}

	m.tracker.Refresh()
	m.tracker.MatchNewSession(sessions)
	m.tracker.MarkActive(sessions)

	justPromoted := computeStateStatuses(sessions, m.tracker, m.state)

	newActiveIDs := make(map[string]bool)
	for _, s := range sessions {
		if s.StateStatus == types.StatusActive {
			newActiveIDs[s.ID] = true
		}
	}

	var namingCmds []tea.Cmd

	// Sessions that just went inactive → immediate naming
	for id := range m.prevActiveIDs {
		if !newActiveIDs[id] {
			content := m.namingContent(id)
			if content != "" {
				namingCmds = append(namingCmds, autoNameCmd(id, content, m.config.AutoNameLines))
			}
		}
	}

	// Newly promoted sessions → delayed naming (30s)
	for _, id := range justPromoted {
		sid := id
		namingCmds = append(namingCmds, tea.Tick(30*time.Second, func(time.Time) tea.Msg {
			return AutoNameTriggerMsg{SessionID: sid}
		}))
	}

	m.prevActiveIDs = newActiveIDs

	// Diff active sessions to update watcher
	if m.watcher != nil {
		newActive := make(map[string]string)
		for _, s := range sessions {
			if s.IsActive && s.FilePath != "" {
				newActive[s.ID] = s.FilePath
			}
		}
		oldActive := make(map[string]string)
		for _, s := range m.sessions {
			if s.IsActive && s.FilePath != "" {
				oldActive[s.ID] = s.FilePath
			}
		}
		for id, path := range newActive {
			if _, was := oldActive[id]; !was {
				_ = m.watcher.Watch(id, path)
			}
		}
		for id, path := range oldActive {
			if _, is := newActive[id]; !is {
				m.watcher.Unwatch(path)
			}
		}
	}

	m.sessions = sessions
	m.projectDirs = project.ScanProjectDirs(m.projectsRoot)
	m.applyFilter()

	if len(namingCmds) > 0 {
		return tea.Batch(namingCmds...)
	}
	return nil
}

func paneCaptureCmd(sessionID, windowID string, lines int) tea.Cmd {
	return func() tea.Msg {
		snap, err := capture.CapturePane(sessionID, windowID, lines)
		return PaneCaptureMsg{Snapshot: snap, Err: err}
	}
}

func paneCaptureTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return PaneCaptureTickMsg{}
	})
}

func (m *Model) captureCmdForSession(sessionID string) tea.Cmd {
	tmuxWindows := m.tracker.TmuxWindowIDs()
	wid, ok := tmuxWindows[sessionID]
	if !ok {
		return nil
	}
	return paneCaptureCmd(sessionID, wid, 30)
}

func cycleValue(current int, values []int) int {
	for i, v := range values {
		if v == current {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}
