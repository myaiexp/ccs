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

	"ccs/internal/config"
	"ccs/internal/project"
	"ccs/internal/session"
	"ccs/internal/types"
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
	sortField    types.SortField
	sortDir      types.SortDir
}

func New(sessions []types.Session, projects []types.Project, cfg *types.Config) Model {
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
	}
	m.applyFilter()

	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case LaunchResumeMsg:
		m.launching = true
		return m, LaunchResume(msg.Session, m.config.ClaudeFlags)

	case LaunchNewMsg:
		m.launching = true
		return m, LaunchNew(msg.Project, m.config.ClaudeFlags)

	case ExecFinishedMsg:
		m.launching = false
		return m, refreshCmd()

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
		switch key {
		case "j", "down":
			// only one pref item for now, but ready for more
			if m.prefsIdx < 0 {
				m.prefsIdx = 0
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

// detailPaneLines calculates the number of lines the detail pane consumes
// for the currently selected session (border + content including wrapped first message).
func (m *Model) detailPaneLines() int {
	if m.focus != FocusSessions || len(m.filtered) == 0 {
		return 0
	}
	s := m.filtered[m.sessionIdx]
	// Base: border(2) + header(1) + blank(1) + project(1) + stats(1) + id(1) = 7
	base := 7
	// Add wrapped first message lines
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
	// Dot
	var prefix string
	if s.IsActive {
		prefix = activeDot
	} else {
		prefix = inactiveDot
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

	// Right side (ctx% + time) — build first so we know exact width
	rightSide := contextStyle(s.ContextPct).Render(ctxStr) + " " + dimStyle.Render(timeStr)
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

	leftSide := fmt.Sprintf("%s %s %-14s  %s", prefix, num, projName, title)
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

	// Status
	status := dimStyle.Render("○ inactive")
	if s.IsActive {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("● active")
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

	lines := []string{
		headerLine,
		"",
		detailLabelStyle.Render("Project ") + dimStyle.Render(s.ProjectDir),
		detailLabelStyle.Render("Messages ") + detailValueStyle.Render(fmt.Sprintf("%d", s.MsgCount)) +
			detailLabelStyle.Render("  Size ") + detailValueStyle.Render(sizeStr) +
			"  " + status + hidden,
		detailLabelStyle.Render("ID ") + dimStyle.Render(s.ID),
	}

	// Full first message, word-wrapped
	if s.FirstMsg != "" {
		lines = append(lines, "")
		wrapped := wrapText(s.FirstMsg, contentWidth)
		for _, wl := range wrapped {
			lines = append(lines, dimStyle.Render(wl))
		}
	}

	content := strings.Join(lines, "\n")
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
		hints = append(hints, "enter/1-9 resume")
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
		"  enter       Resume selected / new in project",
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
		label   string
		enabled bool
	}
	items := []prefItem{
		{"Relative numbers (nvim-style)", m.config.RelativeNumbers},
	}

	lines := []string{
		titleStyle.Render("Preferences"),
		"",
	}
	for i, item := range items {
		toggle := dimStyle.Render("[ ]")
		if item.enabled {
			toggle = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("[✓]")
		}
		label := item.label
		cursor := "  "
		if i == m.prefsIdx {
			cursor = cursorStyle.Render("▸ ")
			label = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(label)
		} else {
			label = dimStyle.Render(label)
		}
		lines = append(lines, fmt.Sprintf("  %s%s %s", cursor, toggle, label))
	}
	lines = append(lines, "", dimStyle.Render("  enter/space toggle  esc/p close"))

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

	active := session.DetectActive()
	session.MarkActiveSessions(sessions, active)

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
