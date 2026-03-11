package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"

	"ccs/internal/activity"
	"ccs/internal/capture"
	"ccs/internal/config"
	"ccs/internal/project"
	"ccs/internal/session"
	"ccs/internal/types"
	"ccs/internal/watcher"
)

// Focus tracks which section has focus.
type Focus int

const (
	FocusSessions Focus = iota
	FocusProjects
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

type Model struct {
	sessions     []types.Session
	filtered     []types.Session
	projects     []types.Project
	filteredProj []types.Project
	config       *types.Config
	filter       textinput.Model
	focus        Focus
	sessionIdx   int
	projectIdx   int
	filtering    bool
	showHidden   bool
	showHelp     bool
	showPrefs    bool
	prefsIdx     int
	confirming   bool
	confirmSess  *types.Session
	width        int
	height       int
	errMsg       string
	pendingG     bool // true when 'g' was pressed, waiting for second 'g'
	sortField    types.SortField
	sortDir      types.SortDir
	tracker      *session.Tracker
	watcher      *watcher.Watcher
	activities   map[string][]activity.Entry    // sessionID -> recent entries
	followID    string                         // session ID being followed (empty = normal)
	paneContent map[string]capture.PaneSnapshot // sessionID -> latest snapshot
}

func New(sessions []types.Session, projects []types.Project, cfg *types.Config, tracker *session.Tracker, w *watcher.Watcher) Model {
	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.Prompt = "/ "
	ti.CharLimit = 64

	// Filter out hidden sessions
	hiddenSet := make(map[string]bool, len(cfg.HiddenSessions))
	for _, id := range cfg.HiddenSessions {
		hiddenSet[id] = true
	}

	m := Model{
		sessions:     sessions,
		projects:     projects,
		filteredProj: filterVisibleProjects(projects, false),
		config:       cfg,
		filter:       ti,
		focus:        FocusSessions,
		sortField:    types.SortByTime,
		sortDir:      types.SortDesc,
		tracker:      tracker,
		watcher:      w,
		activities:   make(map[string][]activity.Entry),
		paneContent:  make(map[string]capture.PaneSnapshot),
	}
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
		// Capture all sessions that have tmux windows (active or not)
		captured := make(map[string]bool)
		tmuxWindows := m.tracker.TmuxWindowIDs()
		for sessID := range tmuxWindows {
			if cmd := m.captureCmdForSession(sessID); cmd != nil {
				cmds = append(cmds, cmd)
				captured[sessID] = true
			}
		}
		// Also capture followed session if not already covered
		if m.followID != "" && !captured[m.followID] {
			if cmd := m.captureCmdForSession(m.followID); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		// Always re-subscribe
		cmds = append(cmds, paneCaptureTickCmd())
		return m, tea.Batch(cmds...)

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

	// ctrl+c always quits
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// Clear any error message on keypress
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
			m.filtering = false
			m.filter.Blur()
			return m, nil
		case "up", "down", "tab":
			return m.handleNavigation(key)
		default:
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			m.applyFilter()
			return m, cmd
		}
	}

	// Help overlay
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	// Preferences overlay
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
			case 0: // relative numbers
				m.config.RelativeNumbers = !m.config.RelativeNumbers
				config.Save(m.config)
			case 1: // activity lines cycle: 3 → 5 → 10 → 15 → 3
				switch m.config.ActivityLines {
				case 3:
					m.config.ActivityLines = 5
				case 5:
					m.config.ActivityLines = 10
				case 10:
					m.config.ActivityLines = 15
				default:
					m.config.ActivityLines = 3
				}
				config.Save(m.config)
			case 2: // name length cycle: 12 → 16 → 20 → 24 → 30 → 12
				switch m.config.ProjectNameMax {
				case 12:
					m.config.ProjectNameMax = 16
				case 16:
					m.config.ProjectNameMax = 20
				case 20:
					m.config.ProjectNameMax = 24
				case 24:
					m.config.ProjectNameMax = 30
				default:
					m.config.ProjectNameMax = 12
				}
				config.Save(m.config)
			}
		case "esc", "p", "q":
			m.showPrefs = false
		}
		return m, nil
	}

	// Delete confirmation
	if m.confirming {
		switch key {
		case "y":
			if m.confirmSess != nil {
				os.Remove(m.confirmSess.FilePath)
				subagentsDir := strings.TrimSuffix(m.confirmSess.FilePath, ".jsonl")
				os.RemoveAll(subagentsDir)
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

	// Handle pending 'g' for gg (go to top)
	if m.pendingG {
		m.pendingG = false
		if key == "g" {
			if m.focus == FocusSessions {
				m.sessionIdx = 0
			} else {
				m.projectIdx = 0
			}
			return m, nil
		}
		// Not 'g' — fall through to normal key handling
	}

	// Number shortcuts: 1-9 launch the Nth session in the sorted list.
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

	case "tab":
		return m.handleNavigation(key)

	case "up", "k":
		return m.handleNavigation("up")

	case "down", "j":
		return m.handleNavigation("down")

	case "G":
		if m.focus == FocusSessions && len(m.filtered) > 0 {
			m.sessionIdx = len(m.filtered) - 1
		} else if m.focus == FocusProjects && len(m.filteredProj) > 0 {
			m.projectIdx = len(m.filteredProj) - 1
		}
		return m, nil

	case "g":
		m.pendingG = true
		return m, nil

	case "left":
		return m.handleNavigation("left")

	case "right":
		return m.handleNavigation("right")

	case "enter":
		return m.handleEnter()

	case "n":
		m.focus = FocusProjects
		return m, nil

	case "d":
		if m.focus == FocusSessions && len(m.filtered) > 0 {
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

	case "x":
		m.toggleHideSession()
		return m, nil

	case "h":
		m.showHidden = !m.showHidden
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
	case "tab":
		if m.focus == FocusSessions {
			m.focus = FocusProjects
			if m.projectIdx >= len(m.filteredProj) {
				m.projectIdx = 0
			}
		} else {
			m.focus = FocusSessions
			if m.sessionIdx >= len(m.filtered) {
				m.sessionIdx = 0
			}
		}

	case "up":
		if m.focus == FocusSessions {
			if m.sessionIdx > 0 {
				m.sessionIdx--
			}
		} else {
			grid := m.projectGrid()
			r, c := gridPosition(grid, m.projectIdx)
			if r > 0 {
				targetRow := grid[r-1]
				if c < len(targetRow) {
					m.projectIdx = targetRow[c]
				} else {
					m.projectIdx = targetRow[len(targetRow)-1]
				}
			}
		}

	case "down":
		if m.focus == FocusSessions {
			if m.sessionIdx < len(m.filtered)-1 {
				m.sessionIdx++
			}
		} else {
			grid := m.projectGrid()
			r, c := gridPosition(grid, m.projectIdx)
			if r < len(grid)-1 {
				targetRow := grid[r+1]
				if c < len(targetRow) {
					m.projectIdx = targetRow[c]
				} else {
					m.projectIdx = targetRow[len(targetRow)-1]
				}
			}
		}

	case "left":
		if m.focus == FocusProjects {
			if m.projectIdx > 0 {
				m.projectIdx--
			}
		}

	case "right":
		if m.focus == FocusProjects {
			if m.projectIdx < len(m.filteredProj)-1 {
				m.projectIdx++
			}
		}
	}
	return m, nil
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == FocusSessions && len(m.filtered) > 0 {
		sess := m.filtered[m.sessionIdx]
		// Check if session has a tmux window — switch to it
		tmuxWindows := m.tracker.TmuxWindowIDs()
		if wid, ok := tmuxWindows[sess.ID]; ok {
			return m, TmuxSwitch(wid)
		}
		// Launch in new tmux window
		return m, TmuxLaunchResume(sess, m.config.ClaudeFlags, m.tracker)
	}
	if m.focus == FocusProjects && len(m.filteredProj) > 0 {
		proj := m.filteredProj[m.projectIdx]
		return m, TmuxLaunchNew(proj, m.config.ClaudeFlags, m.tracker)
	}
	return m, nil
}

func (m Model) handleFollow() (Model, tea.Cmd) {
	if m.focus != FocusSessions || len(m.filtered) == 0 {
		return m, nil
	}
	sess := m.filtered[m.sessionIdx]

	// Toggle off if already following this session
	if m.followID == sess.ID {
		m.followID = ""
		return m, nil
	}

	// Only follow SourceTmux sessions
	if sess.ActiveSource != types.SourceTmux {
		m.errMsg = "Can only follow sessions with tmux windows"
		return m, nil
	}

	m.followID = sess.ID
	return m, m.startPaneCapture(sess.ID)
}

func (m *Model) toggleHideSession() {
	if m.focus != FocusSessions || len(m.filtered) == 0 {
		return
	}
	sess := m.filtered[m.sessionIdx]

	// Toggle hidden status
	found := false
	var newHidden []string
	for _, id := range m.config.HiddenSessions {
		if id == sess.ID {
			found = true
			continue // remove it
		}
		newHidden = append(newHidden, id)
	}
	if !found {
		newHidden = append(newHidden, sess.ID)
	}
	m.config.HiddenSessions = newHidden
	config.Save(m.config)
	m.applyFilter()
}

func (m *Model) applyFilter() {
	// Remember selected session ID to restore after re-sort
	var selectedID string
	if m.sessionIdx < len(m.filtered) {
		selectedID = m.filtered[m.sessionIdx].ID
	}

	query := m.filter.Value()

	hiddenSet := make(map[string]bool, len(m.config.HiddenSessions))
	for _, id := range m.config.HiddenSessions {
		hiddenSet[id] = true
	}

	if query == "" {
		if m.showHidden {
			m.filtered = m.sessions
		} else {
			m.filtered = nil
			for _, s := range m.sessions {
				if !hiddenSet[s.ID] {
					m.filtered = append(m.filtered, s)
				}
			}
		}
		m.filteredProj = filterVisibleProjects(m.projects, m.showHidden)
		m.sortFiltered()
		m.restoreSelection(selectedID)
		return
	}

	// Build source list (respecting hidden)
	var source []types.Session
	if m.showHidden {
		source = m.sessions
	} else {
		for _, s := range m.sessions {
			if !hiddenSet[s.ID] {
				source = append(source, s)
			}
		}
	}

	// Fuzzy filter sessions
	targets := make([]string, len(source))
	for i, s := range source {
		targets[i] = s.ProjectName + " " + s.SessionName + " " + s.Title
	}
	matches := fuzzy.Find(query, targets)
	m.filtered = make([]types.Session, len(matches))
	for i, match := range matches {
		m.filtered[i] = source[match.Index]
	}

	// Fuzzy filter projects
	visible := filterVisibleProjects(m.projects, m.showHidden)
	projTargets := make([]string, len(visible))
	for i, p := range visible {
		projTargets[i] = p.Name
	}
	projMatches := fuzzy.Find(query, projTargets)
	m.filteredProj = make([]types.Project, len(projMatches))
	for i, match := range projMatches {
		m.filteredProj[i] = visible[match.Index]
	}

	m.sortFiltered()
	m.restoreSelection(selectedID)
}

// restoreSelection finds the session with the given ID in the filtered list
// and sets sessionIdx to it. Falls back to clamping if not found.
func (m *Model) restoreSelection(id string) {
	if id != "" {
		for i, s := range m.filtered {
			if s.ID == id {
				m.sessionIdx = i
				m.clampIndices()
				return
			}
		}
	}
	m.clampIndices()
}

func (m *Model) sortFiltered() {
	sort.SliceStable(m.filtered, func(i, j int) bool {
		var less bool
		switch m.sortField {
		case types.SortByTime:
			less = m.filtered[i].LastActive.After(m.filtered[j].LastActive)
		case types.SortByContext:
			less = m.filtered[i].ContextPct > m.filtered[j].ContextPct
		case types.SortBySize:
			less = m.filtered[i].FileSize > m.filtered[j].FileSize
		case types.SortByName:
			less = strings.ToLower(m.filtered[i].Title) < strings.ToLower(m.filtered[j].Title)
		}
		if m.sortDir == types.SortAsc {
			less = !less
		}
		return less
	})
}

func (m *Model) clampIndices() {
	if m.sessionIdx >= len(m.filtered) {
		m.sessionIdx = max(0, len(m.filtered)-1)
	}
	if m.projectIdx >= len(m.filteredProj) {
		m.projectIdx = max(0, len(m.filteredProj)-1)
	}
}

// scrollWindow returns the start and end indices for the visible session window.
func (m *Model) scrollWindow() (int, int) {
	showDetail := m.focus == FocusSessions && len(m.filtered) > 0
	projGridRows := len(m.projectGrid())
	if projGridRows == 0 {
		projGridRows = 1
	}
	fixedOverhead := 8 + projGridRows
	if showDetail {
		// -1 because the detail pane replaces one of the maxRows session slots
		fixedOverhead += m.detailPaneLines() - 1
	}
	availHeight := m.height - 2
	maxRows := availHeight - fixedOverhead
	if maxRows < 3 {
		maxRows = 3
	}
	if maxRows > len(m.filtered) {
		maxRows = len(m.filtered)
	}

	half := maxRows / 2
	start := m.sessionIdx - half
	if start < 0 {
		start = 0
	}
	if start > len(m.filtered)-maxRows {
		start = max(0, len(m.filtered)-maxRows)
	}
	end := start + maxRows
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	return start, end
}


func filterVisibleProjects(projects []types.Project, showHidden bool) []types.Project {
	if showHidden {
		return projects
	}
	var visible []types.Project
	for _, p := range projects {
		if !p.Hidden {
			visible = append(visible, p)
		}
	}
	return visible
}

// gridLayout holds the computed layout of the project grid.
type gridLayout struct {
	names     []string // truncated display names
	cols      int      // number of columns
	rows      int      // number of rows
	colWidths []int    // width of each column
	grid      [][]int  // rows of item indices
}

// computeGridLayout computes the columnar layout for the project grid.
// Used by both projectGrid (for navigation) and renderProjects (for rendering).
func (m *Model) computeGridLayout() *gridLayout {
	if len(m.filteredProj) == 0 {
		return nil
	}

	maxWidth := m.width - 4
	if maxWidth < 40 {
		maxWidth = 40
	}
	nameMax := m.config.ProjectNameMax
	gap := 2
	n := len(m.filteredProj)

	names := make([]string, n)
	for i, p := range m.filteredProj {
		names[i] = truncateName(p.Name, nameMax)
	}

	bestCols := 1
	for cols := n; cols >= 1; cols-- {
		rows := (n + cols - 1) / cols
		totalWidth := 0
		for c := 0; c < cols; c++ {
			colMax := 0
			for r := 0; r < rows; r++ {
				idx := r*cols + c
				if idx < n && len(names[idx]) > colMax {
					colMax = len(names[idx])
				}
			}
			totalWidth += colMax
			if c < cols-1 {
				totalWidth += gap
			}
		}
		if totalWidth <= maxWidth {
			bestCols = cols
			break
		}
	}

	rows := (n + bestCols - 1) / bestCols
	colWidths := make([]int, bestCols)
	for c := 0; c < bestCols; c++ {
		for r := 0; r < rows; r++ {
			idx := r*bestCols + c
			if idx < n && len(names[idx]) > colWidths[c] {
				colWidths[c] = len(names[idx])
			}
		}
	}

	var grid [][]int
	for r := 0; r < rows; r++ {
		var row []int
		for c := 0; c < bestCols; c++ {
			idx := r*bestCols + c
			if idx < n {
				row = append(row, idx)
			}
		}
		grid = append(grid, row)
	}

	return &gridLayout{names: names, cols: bestCols, rows: rows, colWidths: colWidths, grid: grid}
}

// projectGrid returns the grid indices for keyboard navigation.
func (m *Model) projectGrid() [][]int {
	gl := m.computeGridLayout()
	if gl == nil {
		return nil
	}
	return gl.grid
}

// gridPosition returns (row, col) for a given item index in the grid.
func gridPosition(grid [][]int, idx int) (int, int) {
	for r, row := range grid {
		for c, itemIdx := range row {
			if itemIdx == idx {
				return r, c
			}
		}
	}
	return 0, 0
}

func (m *Model) activityLines() int {
	if m.config.ActivityLines > 0 {
		return m.config.ActivityLines
	}
	return 5
}

// detailPaneLines returns a fixed height for the detail pane so that the
// session list doesn't jump around when switching between sessions with
// different amounts of content. Always reserves space for the content section
// (pane capture or activity entries).
// Layout: header(1) + info(1) + blank(1) + status(1) + blank(1) + content(N) + border(2)
func (m *Model) detailPaneLines() int {
	if m.focus != FocusSessions || len(m.filtered) == 0 {
		return 0
	}
	// 7 = header(1) + info(1) + blank(1) + status(1) + blank(1) + border(2)
	return 7 + m.activityLines()
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
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	projectsDir := filepath.Join(home, ".claude", "projects")

	sessions, err := session.DiscoverSessions(projectsDir)
	if err != nil {
		return nil
	}

	// Refresh tracker: prune dead PIDs, seed from /proc
	m.tracker.Refresh()
	m.tracker.MatchNewSession(sessions)

	m.tracker.MarkActive(sessions)

	// Diff active sessions to update watcher
	if m.watcher != nil {
		// Build new active set
		newActive := make(map[string]string) // sessionID → filePath
		for _, s := range sessions {
			if s.IsActive && s.FilePath != "" {
				newActive[s.ID] = s.FilePath
			}
		}

		// Build old active set
		oldActive := make(map[string]string)
		for _, s := range m.sessions {
			if s.IsActive && s.FilePath != "" {
				oldActive[s.ID] = s.FilePath
			}
		}

		// Watch newly-active, unwatch newly-inactive
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
	m.projects = project.DiscoverProjects(sessions, m.config)
	m.applyFilter()
	return nil
}

// paneCaptureCmd creates a tea.Cmd that captures pane content for a session.
func paneCaptureCmd(sessionID, windowID string, lines int) tea.Cmd {
	return func() tea.Msg {
		snap, err := capture.CapturePane(sessionID, windowID, lines)
		return PaneCaptureMsg{Snapshot: snap, Err: err}
	}
}

// paneCaptureTickCmd fires a PaneCaptureTickMsg after 1 second.
func paneCaptureTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return PaneCaptureTickMsg{}
	})
}

// captureCmdForSession returns a paneCaptureCmd for the given session ID,
// looking up the tmux window ID from the tracker. Returns nil if not found.
func (m *Model) captureCmdForSession(sessionID string) tea.Cmd {
	tmuxWindows := m.tracker.TmuxWindowIDs()
	wid, ok := tmuxWindows[sessionID]
	if !ok {
		return nil
	}
	return paneCaptureCmd(sessionID, wid, 30)
}

// startPaneCapture begins polling pane capture, returning the initial commands.
func (m *Model) startPaneCapture(sessionID string) tea.Cmd {
	cmd := m.captureCmdForSession(sessionID)
	if cmd == nil {
		return nil
	}
	return tea.Batch(cmd, paneCaptureTickCmd())
}
