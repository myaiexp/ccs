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

// StatusSummaryTickMsg triggers periodic AI status summaries for active sessions.
type StatusSummaryTickMsg struct{}

// StatusSummaryMsg delivers a periodic status summary result.
type StatusSummaryMsg struct {
	SessionID string
	Status    string
	Err       error
}

// TransitionSummaryMsg delivers the condensed name + comprehensive summary
// when a session goes inactive.
type TransitionSummaryMsg struct {
	SessionID string
	Name      string
	Summary   string
	Err       error
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
	lastSummaryInput  map[string]string              // sessionID -> pane content hash from last status summary call
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
		prevActiveIDs:    make(map[string]bool),
		lastSummaryInput: make(map[string]string),
	}
	_ = computeStateStatuses(m.sessions, m.tracker, m.state)
	m.applyFilter()

	return m
}

func (m Model) Init() tea.Cmd {
	// Initial status summary tick fires quickly (5s) to populate on startup;
	// subsequent ticks are 2 minutes apart (scheduled by the handler).
	initialStatusTick := tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return StatusSummaryTickMsg{}
	})
	cmds := []tea.Cmd{tickCmd(), paneCaptureTickCmd(), initialStatusTick}

	if m.watcher != nil {
		go m.watcher.Run()
		cmds = append(cmds, watchCmd(m.watcher))

		// Watch all currently-active sessions
		for _, s := range m.sessions {
			if s.StateStatus == types.StatusActive && s.FilePath != "" {
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
			m.state.SetName(msg.SessionID, msg.Name, state.NameSourceAuto)
		}
		return m, nil

	case AutoNameTriggerMsg:
		content := m.namingContent(msg.SessionID)
		if content != "" {
			return m, autoNameCmd(msg.SessionID, content, m.config.AutoNameLines)
		}
		return m, nil

	case StatusSummaryTickMsg:
		var cmds []tea.Cmd
		activeCount := 0
		contentCount := 0
		newCount := 0
		for _, s := range m.filtered {
			if s.StateStatus == types.StatusActive {
				activeCount++
				content := m.statusContent(s.ID)
				if content != "" {
					contentCount++
					if m.lastSummaryInput[s.ID] == content {
						continue // content unchanged, skip haiku call
					}
					newCount++
					m.lastSummaryInput[s.ID] = content
					cmds = append(cmds, statusSummaryCmd(s.ID, content, m.config.AutoNameLines))
				}
			}
		}
		if newCount > 0 {
			naming.LogEntry("TICK dispatching=%d (active=%d)", newCount, activeCount)
		}
		cmds = append(cmds, statusSummaryTickCmd())
		return m, tea.Batch(cmds...)

	case StatusSummaryMsg:
		if msg.Status != "" {
			m.state.AppendStatus(msg.SessionID, msg.Status, 20)
		}
		return m, nil

	case TransitionSummaryMsg:
		if msg.Name != "" {
			m.state.SetName(msg.SessionID, msg.Name, state.NameSourceAuto)
		}
		if msg.Summary != "" {
			m.state.SetSummary(msg.SessionID, msg.Summary)
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
				m.state.SetName(m.renameTarget, value, state.NameSourceManual)
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
				computeStateStatuses(m.sessions, m.tracker, m.state)
				m.applyFilter()
			}
		}
		return m, nil

	case "o":
		if len(m.filtered) > 0 {
			sess := m.filtered[m.sessionIdx]
			if sess.StateStatus == types.StatusDone {
				m.state.Reopen(sess.ID)
				computeStateStatuses(m.sessions, m.tracker, m.state)
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
			content := m.statusContent(sess.ID)
			if content != "" {
				return m, statusSummaryCmd(sess.ID, content, m.config.AutoNameLines)
			}
			m.errMsg = "No content available for status summary"
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
		// Project dirs first (for quick new session launch), then sessions
		m.searchResults = nil

		// Match project dirs (shown first)
		dirTargets := make([]string, len(m.projectDirs))
		for i, d := range m.projectDirs {
			dirTargets[i] = d.Name
		}
		dirMatches := fuzzy.Find(query, dirTargets)
		for _, match := range dirMatches {
			if match.Score <= 0 {
				continue
			}
			d := m.projectDirs[match.Index]
			m.searchResults = append(m.searchResults, SearchResult{DirPath: d.Path, DirName: d.Name})
		}

		// Match sessions, then sort: active > open > most recent
		sessTargets := make([]string, len(source))
		for i, s := range source {
			sessTargets[i] = s.ProjectName + " " + m.displayName(s) + " " + s.Title
		}
		sessMatches := fuzzy.Find(query, sessTargets)
		var sessResults []SearchResult
		for _, match := range sessMatches {
			if match.Score <= 0 {
				continue
			}
			s := source[match.Index]
			sessResults = append(sessResults, SearchResult{Session: &s})
		}
		sort.SliceStable(sessResults, func(i, j int) bool {
			si, sj := sessResults[i].Session, sessResults[j].Session
			pi, pj := statePriority(si.StateStatus), statePriority(sj.StateStatus)
			if pi != pj {
				return pi < pj
			}
			return si.LastActive.After(sj.LastActive)
		})
		m.searchResults = append(m.searchResults, sessResults...)

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

	// Active: preserve existing order, insert new sessions at the top by recency.
	// This prevents the list from shuffling under the cursor on every refresh.
	if len(m.filtered) > 0 {
		existingOrder := make(map[string]int)
		for i, s := range m.filtered {
			if s.StateStatus == types.StatusActive {
				existingOrder[s.ID] = i
			}
		}
		// Separate known vs new active sessions
		var known, fresh []types.Session
		for _, s := range active {
			if _, exists := existingOrder[s.ID]; exists {
				known = append(known, s)
			} else {
				fresh = append(fresh, s)
			}
		}
		// Known: preserve previous order
		sort.SliceStable(known, func(i, j int) bool {
			return existingOrder[known[i].ID] < existingOrder[known[j].ID]
		})
		// Fresh: sort by recency
		sort.SliceStable(fresh, func(i, j int) bool {
			return fresh[i].LastActive.After(fresh[j].LastActive)
		})
		// New sessions appear at top
		active = append(fresh, known...)
	} else {
		// First load: sort by recency
		sort.SliceStable(active, func(i, j int) bool {
			return active[i].LastActive.After(active[j].LastActive)
		})
	}

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

// maxActiveStatusLines returns how many status lines each active session may show,
// capped to fit within terminal height alongside other sections.
func (m *Model) maxActiveStatusLines() int {
	active := m.activeSessions()
	openList := m.openSessions()
	nActive := len(active)
	nOpen := len(openList)

	if nActive == 0 || m.height == 0 {
		return 5
	}

	// Non-active overhead: border(2) + title(1) + ACTIVE header+margin(2) +
	// OPEN header+margin(2) + scroll indicator(1) + footer+margin(2) = 10
	overhead := 10

	// Detail pane
	if m.sessionIdx >= nActive && nOpen > 0 {
		overhead += m.detailPaneLines()
	}

	// 1 header line per active session (no status)
	overhead += nActive

	// Reserve at least 1 open row
	if nOpen > 0 {
		overhead++
	}

	avail := m.height - overhead
	if avail <= 0 {
		return 0
	}

	perSession := avail / nActive
	if perSession > 3 {
		perSession = 3
	}
	return perSession
}

// activeRowLines returns how many lines an active row takes.
func (m *Model) activeRowLines(s types.Session) int {
	maxStatus := m.maxActiveStatusLines()
	lines := 1 // header line
	history := m.state.StatusHistory(s.ID)
	if len(history) > 0 {
		n := len(history)
		if n > maxStatus {
			n = maxStatus
		}
		lines += n
	} else if maxStatus > 0 {
		if snap, ok := m.paneContent[s.ID]; ok && snap.Content != "" {
			paneLines := strings.Split(snap.Content, "\n")
			n := 2
			if n > maxStatus {
				n = maxStatus
			}
			if len(paneLines) < n {
				n = len(paneLines)
			}
			lines += n
		}
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
		activeLines += 2 // ACTIVE header + margin
	}

	// Fixed overhead: border(2) + title(1) + OPEN header+margin(2) + scroll indicator(1) + footer+margin(2) = 8
	fixedOverhead := 8 + activeLines

	showDetail := m.sessionIdx >= nActive && nOpen > 0
	if showDetail {
		fixedOverhead += m.detailPaneLines()
	}

	availHeight := m.height - fixedOverhead
	maxRows := availHeight
	if maxRows < 0 {
		maxRows = 0
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

// detailBodyRows returns the number of content rows in the detail pane body.
func (m *Model) detailBodyRows() int {
	return m.activityLines() + 3
}

// detailPaneLines returns the total rendered height of the detail pane (including border).
func (m *Model) detailPaneLines() int {
	if len(m.filtered) == 0 {
		return 0
	}
	// border(2) + header+info+blank(3) + body rows
	return 2 + 3 + m.detailBodyRows()
}

// computeStateStatuses sets StateStatus on each session by merging tracker and state store.
// Returns IDs of sessions that were just promoted to open.
func computeStateStatuses(sessions []types.Session, tracker *session.Tracker, st *state.Store) []string {
	var openIDs map[string]bool
	if tracker != nil {
		openIDs = tracker.ActiveSessionIDs()
	} else {
		openIDs = make(map[string]bool)
	}
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
			case state.StatusOpen:
				sessions[i].StateStatus = types.StatusOpen
			case state.StatusDone:
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

func statusSummaryCmd(sessionID, paneContent string, maxLines int) tea.Cmd {
	return func() tea.Msg {
		result := naming.GenerateStatus(sessionID, paneContent, maxLines)
		return StatusSummaryMsg{SessionID: result.SessionID, Status: result.Name, Err: result.Err}
	}
}

func statusSummaryTickCmd() tea.Cmd {
	return tea.Tick(2*time.Minute, func(time.Time) tea.Msg {
		return StatusSummaryTickMsg{}
	})
}

// statusContent returns the best available content for AI status summary.
// Prefers pane capture, falls back to JSONL conversation text (not raw JSON).
func (m *Model) statusContent(sessionID string) string {
	if snap, ok := m.paneContent[sessionID]; ok && snap.Content != "" {
		return snap.Content
	}
	// Fallback: extract conversation text from JSONL (human + assistant text only)
	for _, s := range m.sessions {
		if s.ID == sessionID && s.FilePath != "" {
			return activity.ExtractConversationText(s.FilePath, 30)
		}
	}
	return ""
}

func transitionSummaryCmd(sessionID string, statusTexts []string) tea.Cmd {
	return func() tea.Msg {
		// Generate condensed name from recent summaries
		nameResult := naming.CondenseName(sessionID, statusTexts)
		// Generate comprehensive summary from all summaries
		summaryResult := naming.GenerateComprehensiveSummary(sessionID, statusTexts)

		return TransitionSummaryMsg{
			SessionID: sessionID,
			Name:      nameResult.Name,
			Summary:   summaryResult.Summary,
		}
	}
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
	sessions, err := session.LoadSessions(m.projectsDir, m.tracker)
	if err != nil {
		m.errMsg = fmt.Sprintf("Session discovery error: %v", err)
		return nil
	}

	justPromoted := computeStateStatuses(sessions, m.tracker, m.state)

	newActiveIDs := make(map[string]bool)
	for _, s := range sessions {
		if s.StateStatus == types.StatusActive {
			newActiveIDs[s.ID] = true
		}
	}

	namingCmds := m.scheduleNamingTriggers(newActiveIDs, justPromoted)
	m.prevActiveIDs = newActiveIDs

	m.syncWatcher(sessions)

	m.sessions = sessions
	m.projectDirs = project.ScanProjectDirs(m.projectsRoot)
	m.eagerLoadActivities()
	m.applyFilter()

	if len(namingCmds) > 0 {
		return tea.Batch(namingCmds...)
	}
	return nil
}

// scheduleNamingTriggers returns naming commands for sessions that just went
// inactive (uses transition summary from status history) or were just promoted (delayed initial status).
func (m *Model) scheduleNamingTriggers(newActiveIDs map[string]bool, justPromoted []string) []tea.Cmd {
	var cmds []tea.Cmd

	// Sessions that just went inactive → generate transition summary from status history
	for id := range m.prevActiveIDs {
		if !newActiveIDs[id] {
			history := m.state.StatusHistory(id)
			if len(history) > 0 {
				texts := make([]string, len(history))
				for i, h := range history {
					texts[i] = h.Text
				}
				cmds = append(cmds, transitionSummaryCmd(id, texts))
			} else {
				// Fallback: no status history, try status summary from conversation text
				content := m.statusContent(id)
				if content != "" {
					cmds = append(cmds, statusSummaryCmd(id, content, m.config.AutoNameLines))
				}
			}
		}
	}

	for _, id := range justPromoted {
		sid := id
		cmds = append(cmds, tea.Tick(30*time.Second, func(time.Time) tea.Msg {
			return AutoNameTriggerMsg{SessionID: sid}
		}))
	}

	return cmds
}

// syncWatcher adds/removes watched files to match the current active session set.
func (m *Model) syncWatcher(sessions []types.Session) {
	if m.watcher == nil {
		return
	}

	newActive := make(map[string]string)
	for _, s := range sessions {
		if s.StateStatus == types.StatusActive && s.FilePath != "" {
			newActive[s.ID] = s.FilePath
		}
	}
	oldActive := make(map[string]string)
	for _, s := range m.sessions {
		if s.StateStatus == types.StatusActive && s.FilePath != "" {
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

// eagerLoadActivities populates m.activities for sessions that have a FilePath
// but no watcher-sourced entries, avoiding I/O inside View().
func (m *Model) eagerLoadActivities() {
	for _, s := range m.sessions {
		if s.FilePath != "" && len(m.activities[s.ID]) == 0 {
			if entries := activity.TailFile(s.FilePath, m.activityLines()); len(entries) > 0 {
				m.activities[s.ID] = entries
			}
		}
	}
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

// statePriority returns sort priority for session states (lower = higher priority).
func statePriority(s types.StateStatus) int {
	switch s {
	case types.StatusActive:
		return 0
	case types.StatusOpen:
		return 1
	default:
		return 2
	}
}

func cycleValue(current int, values []int) int {
	for i, v := range values {
		if v == current {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}
