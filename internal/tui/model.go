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
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"ccs/internal/activity"
	"ccs/internal/config"
	"ccs/internal/project"
	"ccs/internal/session"
	"ccs/internal/tmux"
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
type LaunchResumeMsg struct{ Session types.Session }
type LaunchNewMsg struct{ Project types.Project }
type RefreshMsg struct{}
type ActivityUpdateMsg struct {
	SessionID string
	Entries   []activity.Entry
}
type TickMsg struct{}

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
	launching    bool
	hubMode      bool
	sortField    types.SortField
	sortDir      types.SortDir
	tracker      *session.Tracker
	watcher      *watcher.Watcher
	activities   map[string][]activity.Entry // sessionID -> recent entries
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
		hubMode:      tmux.InTmux(),
		sortField:    types.SortByTime,
		sortDir:      types.SortDesc,
		tracker:      tracker,
		watcher:      w,
		activities:   make(map[string][]activity.Entry),
	}
	m.applyFilter()

	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd()}

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

	case LaunchResumeMsg:
		m.launching = true
		return m, LaunchResume(msg.Session, m.config.ClaudeFlags, m.tracker)

	case LaunchNewMsg:
		m.launching = true
		return m, LaunchNew(msg.Project, m.config.ClaudeFlags, m.tracker)

	case ExecFinishedMsg:
		m.launching = false
		return m, refreshCmd()

	case TmuxLaunchDoneMsg:
		return m, refreshCmd()

	case TmuxSwitchDoneMsg:
		return m, nil

	case ActivityUpdateMsg:
		m.activities[msg.SessionID] = msg.Entries
		if m.watcher != nil {
			return m, watchCmd(m.watcher)
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

	// ctrl+c always quits
	if key == "ctrl+c" {
		return m, tea.Quit
	}

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
		const prefsCount = 2
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

	// Number shortcuts: 1-9 launch the Nth session in the sorted list.
	if key >= "1" && key <= "9" {
		n := int(key[0] - '0')
		idx := n - 1
		if idx < len(m.filtered) {
			sess := m.filtered[idx]
			if m.hubMode {
				tmuxWindows := m.tracker.TmuxWindowIDs()
				if wid, ok := tmuxWindows[sess.ID]; ok {
					return m, TmuxSwitch(wid)
				}
				return m, TmuxLaunchResume(sess, m.config.ClaudeFlags, m.tracker)
			}
			return m, func() tea.Msg {
				return LaunchResumeMsg{Session: sess}
			}
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

	case "left":
		return m.handleNavigation("left")

	case "right":
		return m.handleNavigation("right")

	case "enter":
		return m.handleEnter()

	case "o":
		return m.handleInlineEnter()

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
		m.sortAndFilter()
		return m, nil

	case "r":
		m.sortDir = m.sortDir.Toggle()
		m.sessionIdx = 0
		m.sortAndFilter()
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

	case "esc":
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
	if m.hubMode {
		return m.handleHubEnter()
	}
	return m.handleInlineEnter()
}

func (m Model) handleHubEnter() (Model, tea.Cmd) {
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

func (m Model) handleInlineEnter() (Model, tea.Cmd) {
	if m.focus == FocusSessions && len(m.filtered) > 0 {
		sess := m.filtered[m.sessionIdx]
		return m, func() tea.Msg {
			return LaunchResumeMsg{Session: sess}
		}
	}
	if m.focus == FocusProjects && len(m.filteredProj) > 0 {
		proj := m.filteredProj[m.projectIdx]
		return m, func() tea.Msg {
			return LaunchNewMsg{Project: proj}
		}
	}
	return m, nil
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
		m.clampIndices()
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
		targets[i] = s.ProjectName + " " + s.Title
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
	m.clampIndices()
}

func (m *Model) sortAndFilter() {
	m.applyFilter()
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
		fixedOverhead += m.detailPaneLines()
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

// projectGrid computes the actual row layout of the project grid,
// matching how renderProjects wraps items by width.
// Returns rows where each row is a slice of item indices.
func (m *Model) projectGrid() [][]int {
	if len(m.filteredProj) == 0 {
		return nil
	}

	maxWidth := m.width - 4
	if maxWidth < 40 {
		maxWidth = 40
	}

	sepWidth := 3 // " · "
	var rows [][]int
	var currentRow []int
	lineWidth := 0

	for i, p := range m.filteredProj {
		nameWidth := lipgloss.Width(p.Name)
		addition := nameWidth
		if len(currentRow) > 0 {
			addition += sepWidth
		}

		if lineWidth+addition > maxWidth && len(currentRow) > 0 {
			rows = append(rows, currentRow)
			currentRow = []int{i}
			lineWidth = nameWidth
		} else {
			currentRow = append(currentRow, i)
			lineWidth += addition
		}
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}

	return rows
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

// detailPaneLines calculates the number of lines the detail pane consumes
// for the currently selected session (border + content including wrapped first message).
func (m *Model) detailPaneLines() int {
	if m.focus != FocusSessions || len(m.filtered) == 0 {
		return 0
	}
	s := m.filtered[m.sessionIdx]

	entries := m.activities[s.ID]
	usesTwoColumn := s.IsActive && len(entries) > 0

	if usesTwoColumn {
		// Two-column layout: height is max(left, right) + border(2)
		contentWidth := m.width - 8
		if contentWidth < 40 {
			contentWidth = 40
		}
		leftWidth := contentWidth * 40 / 100
		if leftWidth < 30 {
			leftWidth = 30
		}

		// Left column: header(1) + blank(1) + project(1) + stats(1) + id(1) = 5 lines
		leftLines := 5
		if s.FirstMsg != "" {
			msgLines := wrapText(s.FirstMsg, leftWidth)
			leftLines += len(msgLines) + 1 // +1 for blank line
		}

		// Right column: "Activity" header(1) + blank(1) + entries
		actCount := m.activityLines()
		if actCount > len(entries) {
			actCount = len(entries)
		}
		rightLines := 2 + actCount // header + blank + entries

		contentLines := leftLines
		if rightLines > contentLines {
			contentLines = rightLines
		}
		return contentLines + 2 // +2 for border
	}

	// Single-column: border(2) + header(1) + blank(1) + project(1) + stats(1) + id(1) = 7
	base := 7
	if s.FirstMsg != "" {
		contentWidth := m.width - 8 // outer border(2) + padding(2) + detail border(2) + detail padding(2)
		if contentWidth < 40 {
			contentWidth = 40
		}
		msgLines := wrapText(s.FirstMsg, contentWidth)
		base += len(msgLines) + 1 // +1 for blank line before message
	}
	return base
}

// View renders the full TUI.
func (m Model) View() string {
	if m.launching {
		return ""
	}

	if m.showHelp {
		return m.renderHelp()
	}

	if m.showPrefs {
		return m.renderPrefs()
	}

	availHeight := m.height - 2 // outer border

	var sections []string

	// Title + filter + sort indicator
	header := titleStyle.Render("ccs")
	if m.filtering || m.filter.Value() != "" {
		header += "  " + m.filter.View()
	}
	sortIndicator := dimStyle.Render(fmt.Sprintf("  sort: %s %s", m.sortField, m.sortDir))
	header += sortIndicator
	sections = append(sections, header)

	// Sessions header
	showDetail := m.focus == FocusSessions && len(m.filtered) > 0
	sessCount := dimStyle.Render(fmt.Sprintf(" (%d)", len(m.filtered)))
	sessHeader := sectionStyle.Render("SESSIONS") + sessCount
	sections = append(sections, sessHeader)

	// Calculate how many session rows fit.
	// Count actual lines consumed by non-session sections:
	// header(1) + sess header with margin(2) + scroll indicator(1) +
	// proj header with margin(2) + footer with margin(2) = 8 fixed
	// Plus project grid rows (estimate from actual data)
	projGridRows := len(m.projectGrid())
	if projGridRows == 0 {
		projGridRows = 1
	}
	fixedOverhead := 8 + projGridRows
	if showDetail {
		fixedOverhead += m.detailPaneLines()
	}
	maxRows := availHeight - fixedOverhead
	if maxRows < 3 {
		maxRows = 3
	}
	if maxRows > len(m.filtered) {
		maxRows = len(m.filtered)
	}

	// Center-scroll: keep selection in the middle of the visible window,
	// except at the start/end of the list where it naturally pins.
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

	if len(m.filtered) == 0 {
		sections = append(sections, dimStyle.Render("  no sessions"))
	} else {
		for i := start; i < end; i++ {
			s := m.filtered[i]
			if showDetail && i == m.sessionIdx {
				sections = append(sections, m.renderDetail(s))
			} else {
				sections = append(sections, m.renderSession(i+1, s))
			}
		}
		// Scroll position indicator
		if len(m.filtered) > maxRows {
			indicator := dimStyle.Render(fmt.Sprintf("  ── %d/%d ──", m.sessionIdx+1, len(m.filtered)))
			sections = append(sections, indicator)
		}
	}

	// Projects
	projHeader := sectionStyle.Render("PROJECTS")
	sections = append(sections, projHeader)
	sections = append(sections, m.renderProjects())

	// Footer / confirmation
	if m.confirming && m.confirmSess != nil {
		title := m.confirmSess.Title
		if len(title) > 40 {
			title = title[:39] + "…"
		}
		confirm := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			MarginTop(1).
			Render(fmt.Sprintf("Delete \"%s\"? [y/n]", title))
		sections = append(sections, confirm)
	} else {
		sections = append(sections, m.renderFooter())
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.width > 0 {
		borderStyle = borderStyle.Width(m.width - 2)
	}

	return borderStyle.Render(content)
}

// renderSession renders a non-selected session row. visNum is the window-local
// shortcut number (1-9+), not the global index.
func (m Model) renderSession(visNum int, s types.Session) string {
	// Three-state dot based on ActiveSource
	var dot string
	switch s.ActiveSource {
	case types.SourceTmux:
		dot = activeDot
	case types.SourceProc:
		dot = externalDot
	default:
		dot = inactiveDot
	}

	// Position number, right-aligned to 4 digits
	numStr := fmt.Sprintf("%4d", visNum)
	num := numStyle.Render(numStr)

	// Project name (truncate if needed)
	projName := s.ProjectName
	if len(projName) > 14 {
		projName = projName[:13] + "…"
	}

	// Context %
	ctxStr := fmt.Sprintf("%d%%", s.ContextPct)

	// Time
	timeStr := formatDuration(s.LastActive)

	// Hidden label (only visible in show-hidden mode)
	hiddenLabel := ""
	if m.showHidden {
		for _, id := range m.config.HiddenSessions {
			if id == s.ID {
				hiddenLabel = dimStyle.Render("[hidden] ")
				break
			}
		}
	}

	// Activity text for active sessions
	activityText := ""
	if entries, ok := m.activities[s.ID]; ok && len(entries) > 0 {
		activityText = activityStyle.Render(activity.FormatEntry(entries[0]))
	}

	// Right side: activity + ctx% + time
	rightSide := contextStyle(s.ContextPct).Render(ctxStr) + " " + dimStyle.Render(timeStr)
	if activityText != "" {
		rightSide = activityText + "  " + rightSide
	}
	if hiddenLabel != "" {
		rightSide = hiddenLabel + rightSide
	}
	rightWidth := lipgloss.Width(rightSide)

	// Left side fixed parts: dot(1) + space(1) + num(4) + space(1) + proj(14) + gap(2) = 23
	leftFixed := 23
	// Content area inside outer border: width - border(2) - padding(2) = width - 4
	contentWidth := m.width - 4
	// Title gets whatever space remains, minus gap(2) before right side
	maxTitle := contentWidth - leftFixed - rightWidth - 2
	if maxTitle < 10 {
		maxTitle = 10
	}

	title := s.Title
	if lipgloss.Width(title) > maxTitle {
		// Truncate by runes to handle multi-byte chars
		for lipgloss.Width(title) > maxTitle-1 && len(title) > 0 {
			title = title[:len(title)-1]
		}
		title += "…"
	}

	leftSide := fmt.Sprintf("%s %s %-14s  %s", dot, num, projName, title)
	gap := contentWidth - lipgloss.Width(leftSide) - rightWidth
	if gap < 1 {
		gap = 1
	}
	line := leftSide + strings.Repeat(" ", gap) + rightSide

	return line
}

func (m Model) renderDetail(s types.Session) string {
	// outer border(2)+padding(2) + detail border(2) = 6 for total rendered width
	// detail padding(2) further reduces content area since Width() includes padding
	detailWidth := m.width - 6 // passed to .Width() (includes detail padding)
	contentWidth := detailWidth - 2 // actual text area (excludes detail padding)
	if detailWidth < 40 {
		detailWidth = 40
	}
	if contentWidth < 38 {
		contentWidth = 38
	}

	// Status with three-state dot
	var status string
	switch s.ActiveSource {
	case types.SourceTmux:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("● active (tmux)")
	case types.SourceProc:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("● active (external)")
	default:
		status = dimStyle.Render("○ inactive")
	}

	// Hidden?
	hidden := ""
	for _, id := range m.config.HiddenSessions {
		if id == s.ID {
			hidden = dimStyle.Render("  [hidden]")
			break
		}
	}

	// Header line: project + title + right-aligned ctx% and time
	ctxPart := contextStyle(s.ContextPct).Render(fmt.Sprintf("%d%%", s.ContextPct))
	timePart := dimStyle.Render(formatDuration(s.LastActive))
	rightSide := ctxPart + " " + timePart
	rightWidth := lipgloss.Width(rightSide)

	projName := s.ProjectName
	if len(projName) > 14 {
		projName = projName[:13] + "…"
	}
	projPart := detailValueStyle.Render(projName) + "  "
	projWidth := lipgloss.Width(projPart)

	// Truncate title to leave room for right side (ctx% + time) with at least 2 gap chars
	maxTitleWidth := contentWidth - projWidth - rightWidth - 2
	if maxTitleWidth < 10 {
		maxTitleWidth = 10
	}
	title := s.Title
	if lipgloss.Width(title) > maxTitleWidth {
		for lipgloss.Width(title) > maxTitleWidth-1 && len(title) > 0 {
			title = title[:len(title)-1]
		}
		title += "…"
	}
	titlePart := detailValueStyle.Render(title)
	leftSide := projPart + titlePart

	gap := contentWidth - lipgloss.Width(leftSide) - rightWidth
	if gap < 1 {
		gap = 1
	}
	headerLine := leftSide + strings.Repeat(" ", gap) + rightSide

	// File size
	sizeStr := formatSize(s.FileSize)

	infoLines := []string{
		headerLine,
		"",
		detailLabelStyle.Render("Project ") + dimStyle.Render(s.ProjectDir),
		detailLabelStyle.Render("Messages ") + detailValueStyle.Render(fmt.Sprintf("%d", s.MsgCount)) +
			detailLabelStyle.Render("  Size ") + detailValueStyle.Render(sizeStr) +
			"  " + status + hidden,
		detailLabelStyle.Render("ID ") + dimStyle.Render(s.ID),
	}

	// Full first message, word-wrapped
	entries := m.activities[s.ID]
	usesTwoColumn := s.IsActive && len(entries) > 0

	if usesTwoColumn {
		// Two-column layout: left (~40%) = info + first message, right (~60%) = activity log
		leftWidth := contentWidth * 40 / 100
		rightColWidth := contentWidth - leftWidth - 2 // 2 for gap between columns
		if leftWidth < 30 {
			leftWidth = 30
		}
		if rightColWidth < 20 {
			rightColWidth = 20
		}

		// Left column: info lines + first message
		var leftLines []string
		leftLines = append(leftLines, infoLines...)
		if s.FirstMsg != "" {
			leftLines = append(leftLines, "")
			wrapped := wrapText(s.FirstMsg, leftWidth)
			for _, wl := range wrapped {
				leftLines = append(leftLines, dimStyle.Render(wl))
			}
		}

		// Right column: activity log
		maxEntries := m.activityLines()
		if maxEntries > len(entries) {
			maxEntries = len(entries)
		}
		var rightLines []string
		rightLines = append(rightLines, detailLabelStyle.Render("Activity"))
		rightLines = append(rightLines, "")
		for i := 0; i < maxEntries; i++ {
			rightLines = append(rightLines, activityStyle.Render(activity.FormatEntry(entries[i])))
		}

		// Pad shorter column to match heights
		leftContent := strings.Join(leftLines, "\n")
		rightContent := strings.Join(rightLines, "\n")

		leftCol := lipgloss.NewStyle().Width(leftWidth).Render(leftContent)
		rightCol := lipgloss.NewStyle().Width(rightColWidth).Render(rightContent)

		content := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "  ", rightCol)
		styled := detailBorderStyle.Width(detailWidth).Render(content)
		return styled
	}

	// Single-column layout (inactive or no activity entries)
	if s.FirstMsg != "" {
		infoLines = append(infoLines, "")
		wrapped := wrapText(s.FirstMsg, contentWidth)
		for _, wl := range wrapped {
			infoLines = append(infoLines, dimStyle.Render(wl))
		}
	}

	content := strings.Join(infoLines, "\n")
	styled := detailBorderStyle.Width(detailWidth).Render(content)
	return styled
}

// wrapText wraps text to fit within maxWidth, respecting existing newlines.
func wrapText(s string, maxWidth int) []string {
	if maxWidth < 10 {
		maxWidth = 10
	}
	var result []string
	for _, paragraph := range strings.Split(s, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len(line)+1+len(w) > maxWidth {
				result = append(result, line)
				line = w
			} else {
				line += " " + w
			}
		}
		result = append(result, line)
	}
	return result
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.0f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func (m Model) renderProjects() string {
	if len(m.filteredProj) == 0 {
		return dimStyle.Render("  no projects")
	}

	var parts []string
	for i, p := range m.filteredProj {
		name := p.Name
		if m.focus == FocusProjects && i == m.projectIdx {
			parts = append(parts, selectedProjectStyle.Render(name))
		} else if p.Hidden {
			parts = append(parts, hiddenProjectStyle.Render(name))
		} else {
			parts = append(parts, normalProjectStyle.Render(name))
		}
	}

	// Join with separators and wrap
	sep := dimStyle.Render(" · ")
	maxWidth := m.width - 4
	if maxWidth < 40 {
		maxWidth = 40
	}

	var lines []string
	var currentLine string
	for i, part := range parts {
		addition := part
		if i > 0 {
			addition = sep + part
		}
		testLen := lipgloss.Width(currentLine + addition)
		if testLen > maxWidth && currentLine != "" {
			lines = append(lines, "  "+currentLine)
			currentLine = part
		} else {
			if currentLine == "" {
				currentLine = part
			} else {
				currentLine += sep + part
			}
		}
	}
	if currentLine != "" {
		lines = append(lines, "  "+currentLine)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderFooter() string {
	var hints []string
	if m.focus == FocusSessions && len(m.filtered) > 0 {
		if m.hubMode {
			hints = append(hints, "enter switch/resume", "o inline")
		} else {
			hints = append(hints, "enter/1-9 resume")
		}
	}
	if m.focus == FocusProjects && len(m.filteredProj) > 0 {
		hints = append(hints, "enter new")
	}
	hints = append(hints, "n new", "/ search", "tab switch", "s sort", "r reverse")
	if m.focus == FocusSessions {
		hints = append(hints, "x hide")
	}
	if m.showHidden {
		hints = append(hints, "h hide hidden")
	} else {
		hints = append(hints, "h show hidden")
	}
	hints = append(hints, "p prefs", "? help", "q quit")
	return footerStyle.Render(strings.Join(hints, "  "))
}

func (m Model) renderHelp() string {
	help := strings.Join([]string{
		titleStyle.Render("ccs — Claude Code Sessions"),
		"",
		"  1-9         Resume session by number",
		"  enter       Resume/switch (tmux) or inline launch",
		"  o           Force inline launch (TUI suspends)",
		"  n           Jump to projects section",
		"  /           Toggle filter bar",
		"  esc         Clear filter / exit filter",
		"  tab         Switch: sessions ↔ projects",
		"  j/k ↑/↓     Navigate (↑↓←→ in projects)",
		"  s           Cycle sort: time → ctx% → size → name",
		"  r           Reverse sort direction",
		"  d           Delete session (with confirm)",
		"  x           Hide/unhide session",
		"  h           Toggle showing hidden items",
		"  p           Preferences",
		"  ?           Toggle this help",
		"  q / ctrl+c  Quit",
	}, "\n")

	styled := helpStyle.Render(help)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, styled)
	}
	return styled
}

func (m Model) renderPrefs() string {
	type prefItem struct {
		label string
		value string // non-empty for cycle items, empty for toggle
		on    bool   // only for toggles
	}
	items := []prefItem{
		{"Relative numbers (nvim-style)", "", m.config.RelativeNumbers},
		{"Activity lines", fmt.Sprintf("%d", m.config.ActivityLines), false},
	}

	lines := []string{
		titleStyle.Render("Preferences"),
		"",
	}
	for i, item := range items {
		cursor := "  "
		if i == m.prefsIdx {
			cursor = cursorStyle.Render("▸ ")
		}

		var indicator string
		label := item.label
		if item.value != "" {
			// Cycle item
			indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Render("[" + item.value + "]")
		} else {
			// Toggle item
			indicator = dimStyle.Render("[ ]")
			if item.on {
				indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("[✓]")
			}
		}

		if i == m.prefsIdx {
			label = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(label)
		} else {
			label = dimStyle.Render(label)
		}
		lines = append(lines, fmt.Sprintf("  %s%s %s", cursor, indicator, label))
	}
	lines = append(lines, "", dimStyle.Render("  enter/space toggle/cycle  esc/p close"))

	content := strings.Join(lines, "\n")
	styled := helpStyle.Render(content)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, styled)
	}
	return styled
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

	// Mark sessions as open based on tracker, with ActiveSource
	openIDs := m.tracker.OpenSessionIDs()
	tmuxWindows := m.tracker.TmuxWindowIDs()
	for i := range sessions {
		if openIDs[sessions[i].ID] {
			sessions[i].IsActive = true
			if _, hasTmux := tmuxWindows[sessions[i].ID]; hasTmux {
				sessions[i].ActiveSource = types.SourceTmux
			} else {
				sessions[i].ActiveSource = types.SourceProc
			}
		}
	}

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
				delete(m.activities, id)
			}
		}
	}

	m.sessions = sessions
	m.projects = project.DiscoverProjects(sessions, m.config)
	m.applyFilter()
	return nil
}

func formatDuration(t time.Time) string {
	d := time.Since(t)

	if d < 60*time.Second {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}
