package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

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

// Messages for launching sessions (wired in Task 7).
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
	confirming   bool
	confirmSess  *types.Session
	width        int
	height       int
	launching    bool
}

func New(sessions []types.Session, projects []types.Project, cfg *types.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.Prompt = "/ "
	ti.CharLimit = 64

	m := Model{
		sessions:     sessions,
		filtered:     sessions,
		projects:     projects,
		filteredProj: filterVisibleProjects(projects, false),
		config:       cfg,
		filter:       ti,
		focus:        FocusSessions,
	}

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
			// Accept filter and switch to navigation
			m.filtering = false
			m.filter.Blur()
			return m, nil
		case "up", "down", "tab":
			// Allow navigation while filtering
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

	// Delete confirmation
	if m.confirming {
		switch key {
		case "y":
			if m.confirmSess != nil {
				// Delete the JSONL and subagents dir
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

	// Number shortcuts for sessions
	if key >= "1" && key <= "9" {
		idx := int(key[0]-'0') - 1
		if idx < len(m.filtered) {
			return m, func() tea.Msg {
				return LaunchResumeMsg{Session: m.filtered[idx]}
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

	case "h":
		m.showHidden = !m.showHidden
		m.applyFilter()
		return m, nil

	case "?":
		m.showHelp = !m.showHelp
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

func (m Model) handleNavigation(key string) (tea.Model, tea.Cmd) {
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
			if m.projectIdx > 0 {
				m.projectIdx--
			}
		}
	case "down":
		if m.focus == FocusSessions {
			if m.sessionIdx < len(m.filtered)-1 {
				m.sessionIdx++
			}
		} else {
			if m.projectIdx < len(m.filteredProj)-1 {
				m.projectIdx++
			}
		}
	}
	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
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

func (m *Model) applyFilter() {
	query := m.filter.Value()

	if query == "" {
		m.filtered = m.sessions
		m.filteredProj = filterVisibleProjects(m.projects, m.showHidden)
		m.clampIndices()
		return
	}

	// Fuzzy filter sessions
	targets := make([]string, len(m.sessions))
	for i, s := range m.sessions {
		targets[i] = s.ProjectName + " " + s.Title
	}
	matches := fuzzy.Find(query, targets)
	m.filtered = make([]types.Session, len(matches))
	for i, match := range matches {
		m.filtered[i] = m.sessions[match.Index]
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

	m.clampIndices()
}

func (m *Model) clampIndices() {
	if m.sessionIdx >= len(m.filtered) {
		m.sessionIdx = max(0, len(m.filtered)-1)
	}
	if m.projectIdx >= len(m.filteredProj) {
		m.projectIdx = max(0, len(m.filteredProj)-1)
	}
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

// View renders the full TUI.
func (m Model) View() string {
	if m.launching {
		return ""
	}

	if m.showHelp {
		return m.renderHelp()
	}

	availHeight := m.height - 2 // border

	var sections []string

	// Title + filter
	header := titleStyle.Render("ccs")
	if m.filtering || m.filter.Value() != "" {
		header += "  " + m.filter.View()
	}
	sections = append(sections, header)

	// Sessions
	sessHeader := sectionStyle.Render("SESSIONS")
	sections = append(sections, sessHeader)

	// Calculate how many sessions we can show
	// Reserve lines for: header(1) + sessions header(1+margin) + projects header(1+margin) + project row(1) + footer(1+margin) + border(2)
	maxSessions := availHeight - 9
	if maxSessions < 3 {
		maxSessions = 3
	}
	if maxSessions > len(m.filtered) {
		maxSessions = len(m.filtered)
	}

	// Determine scroll window
	start := 0
	if m.sessionIdx >= maxSessions {
		start = m.sessionIdx - maxSessions + 1
	}
	end := start + maxSessions
	if end > len(m.filtered) {
		end = len(m.filtered)
		start = max(0, end-maxSessions)
	}

	if len(m.filtered) == 0 {
		sections = append(sections, dimStyle.Render("  no sessions"))
	} else {
		for i := start; i < end; i++ {
			s := m.filtered[i]
			sections = append(sections, m.renderSession(i, s))
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

func (m Model) renderSession(idx int, s types.Session) string {
	// Dot
	dot := inactiveDot
	if s.IsActive {
		dot = activeDot
	}

	// Number (1-indexed)
	num := numStyle.Render(fmt.Sprintf("[%d]", idx+1))

	// Project name (truncate if needed)
	projName := s.ProjectName
	if len(projName) > 14 {
		projName = projName[:13] + "…"
	}

	// Title (truncate)
	title := s.Title
	maxTitle := m.width - 40
	if maxTitle < 20 {
		maxTitle = 20
	}
	if len(title) > maxTitle {
		title = title[:maxTitle-1] + "…"
	}

	// Context %
	ctxStr := contextStyle(s.ContextPct).Render(fmt.Sprintf("%d%%", s.ContextPct))

	// Time
	timeStr := dimStyle.Render(formatDuration(s.LastActive))

	line := fmt.Sprintf("%s %s %-14s  %-*s  %4s %s",
		dot, num, projName, maxTitle, title, ctxStr, timeStr)

	if m.focus == FocusSessions && idx == m.sessionIdx {
		return selectedStyle.Render(line)
	}
	return line
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
		// Use rough length (strip ANSI is complex, approximate)
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
	hints = append(hints, "n new", "/ search", "tab switch")
	if m.showHidden {
		hints = append(hints, "h hide")
	} else {
		hints = append(hints, "h show hidden")
	}
	hints = append(hints, "? help", "q quit")
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
		"  j/k ↑/↓     Navigate",
		"  d           Mark session for deletion",
		"  h           Toggle hidden projects",
		"  ?           Toggle this help",
		"  q / ctrl+c  Quit",
	}, "\n")

	styled := helpStyle.Render(help)

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

	activeDirs := session.DetectActive()

	// Mark active sessions
	for i := range sessions {
		s := &sessions[i]
		for dir := range activeDirs {
			_, absPath := session.DecodeProjectDir(dir)
			if absPath == s.ProjectDir {
				s.IsActive = true
				break
			}
		}
	}

	m.sessions = sessions
	m.projects = project.DiscoverProjects(sessions, activeDirs, m.config)
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
