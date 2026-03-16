package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ccs/internal/activity"
	"ccs/internal/types"
)

// View renders the full TUI.
func (m Model) View() string {
	if m.showHelp {
		return m.renderHelp()
	}

	if m.showPrefs {
		return m.renderPrefs()
	}

	if m.followID != "" {
		return m.renderFollowView()
	}

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
	// Active count
	activeCount := 0
	for _, s := range m.filtered {
		if s.IsActive {
			activeCount++
		}
	}
	if activeCount > 0 {
		sessHeader += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render(fmt.Sprintf("%d active", activeCount))
	}
	sections = append(sections, sessHeader)

	start, end := m.scrollWindow()
	maxRows := end - start

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

	// Footer / confirmation / error
	if m.errMsg != "" {
		errLine := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			MarginTop(1).
			Render(m.errMsg)
		sections = append(sections, errLine)
	} else if m.confirming && m.confirmSess != nil {
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

	bs := borderStyle
	if m.width > 0 {
		bs = bs.Width(m.width - 2)
	}

	return bs.Render(content)
}

// renderFollowView renders the split layout: compressed session list + live pane output.
func (m Model) renderFollowView() string {
	var sections []string

	// Header
	header := titleStyle.Render("ccs")
	sortIndicator := dimStyle.Render(fmt.Sprintf("  sort: %s %s", m.sortField, m.sortDir))
	header += sortIndicator
	sections = append(sections, header)

	// Compressed session list — show up to 8 rows without detail pane
	sessCount := dimStyle.Render(fmt.Sprintf(" (%d)", len(m.filtered)))
	sessHeader := sectionStyle.Render("SESSIONS") + sessCount
	sections = append(sections, sessHeader)

	// Calculate how many session rows fit in top ~40%
	topRows := (m.height * 40 / 100) - 4 // header(1) + sessHeader(1) + footer(1) + border(1)
	if topRows < 3 {
		topRows = 3
	}
	if topRows > 8 {
		topRows = 8
	}
	if topRows > len(m.filtered) {
		topRows = len(m.filtered)
	}

	// Center scroll around selected
	half := topRows / 2
	start := m.sessionIdx - half
	if start < 0 {
		start = 0
	}
	if start > len(m.filtered)-topRows {
		start = max(0, len(m.filtered)-topRows)
	}
	end := start + topRows
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	if len(m.filtered) == 0 {
		sections = append(sections, dimStyle.Render("  no sessions"))
	} else {
		for i := start; i < end; i++ {
			sections = append(sections, m.renderSession(i+1, m.filtered[i]))
		}
	}

	// Follow pane — bottom portion
	var followedSess *types.Session
	for _, s := range m.filtered {
		if s.ID == m.followID {
			sess := s
			followedSess = &sess
			break
		}
	}

	contentWidth := m.width - 6 // outer border(2) + padding(2) + pane border(2)
	if contentWidth < 40 {
		contentWidth = 40
	}
	paneWidth := contentWidth - 2 // pane padding

	// Pane header
	paneTitle := "Following: "
	if followedSess != nil {
		paneTitle += followedSess.ProjectName + " — " + followedSess.Title
	} else {
		paneTitle += m.followID
	}
	if lipgloss.Width(paneTitle) > paneWidth {
		paneTitle = paneTitle[:paneWidth-1] + "…"
	}
	paneHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render(paneTitle)

	// Pane content
	paneText := dimStyle.Render("Waiting for capture...")
	if snap, ok := m.paneContent[m.followID]; ok && snap.Content != "" {
		// Show last N lines that fit, bottom-anchored
		paneLines := strings.Split(snap.Content, "\n")
		availPaneRows := m.height - len(sections) - 6 // footer(1) + pane border(2) + pane header(2) + margin(1)
		if availPaneRows < 3 {
			availPaneRows = 3
		}
		if len(paneLines) > availPaneRows {
			paneLines = paneLines[len(paneLines)-availPaneRows:]
		}
		paneText = strings.Join(paneLines, "\n")
	}

	paneContent := paneHeader + "\n" + paneText
	paneStyle := followPaneStyle
	if contentWidth > 0 {
		paneStyle = paneStyle.Width(contentWidth)
	}
	sections = append(sections, paneStyle.Render(paneContent))

	// Follow mode footer
	followFooter := footerStyle.Render("f unfollow  esc exit  enter switch  / search  ? help  q quit")
	sections = append(sections, followFooter)

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	bs := borderStyle
	if m.width > 0 {
		bs = bs.Width(m.width - 2)
	}

	return bs.Render(content)
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

	// Project name (natural width, truncate only if very long)
	projName := s.ProjectName
	if len(projName) > 20 {
		projName = projName[:19] + "…"
	}

	// Session name (from /session-name)
	sessName := ""
	if s.SessionName != "" {
		sessName = s.SessionName
		if len(sessName) > 30 {
			sessName = sessName[:29] + "…"
		}
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

	// Activity text for active sessions (raw, will be truncated below)
	rawActivity := ""
	if entries, ok := m.activities[s.ID]; ok && len(entries) > 0 {
		rawActivity = activity.FormatEntry(entries[0])
	}

	// Right side (fixed part): ctx% + time + optional hidden label
	fixedRight := contextStyle(s.ContextPct).Render(ctxStr) + " " + dimStyle.Render(timeStr)
	if hiddenLabel != "" {
		fixedRight = hiddenLabel + fixedRight
	}
	fixedRightWidth := lipgloss.Width(fixedRight)

	// Left side: dot(1) + space(1) + num(4) + space(1) + proj(natural) + gap(2) [+ sessName + gap(2)]
	projWidth := lipgloss.Width(projName)
	sessNameWidth := 0
	sessNamePart := ""
	if sessName != "" {
		sessNamePart = lipgloss.NewStyle().Foreground(lipgloss.Color("183")).Render(sessName)
		sessNameWidth = lipgloss.Width(sessNamePart) + 2 // + gap before title
	}
	leftFixed := 7 + projWidth + 2 + sessNameWidth // dot+space+num+space + proj + gap + sessName + gap
	// Content area inside outer border: width - border(2) - padding(2) = width - 4
	contentWidth := m.width - 4

	// Budget: contentWidth = leftFixed + title + gap(2) + [activity + gap(2)] + fixedRight
	// Allocate at least 10 chars for title, then activity gets the rest
	activityText := ""
	if rawActivity != "" {
		maxActivity := contentWidth - leftFixed - fixedRightWidth - 10 - 6 // 10=minTitle, 6=gaps
		if maxActivity >= 15 {
			if lipgloss.Width(rawActivity) > maxActivity {
				rawActivity = truncateToWidth(rawActivity, maxActivity)
			}
			activityText = activityStyle.Render(rawActivity)
		}
	}

	rightSide := fixedRight
	if activityText != "" {
		rightSide = activityText + "  " + rightSide
	}
	rightWidth := lipgloss.Width(rightSide)

	// Title gets whatever space remains, minus gap(2) before right side
	maxTitle := contentWidth - leftFixed - rightWidth - 2
	if maxTitle < 10 {
		maxTitle = 10
	}

	title := truncateToWidth(s.Title, maxTitle)

	var leftSide string
	if sessName != "" {
		leftSide = fmt.Sprintf("%s %s %s  %s  %s", dot, num, projName, sessNamePart, title)
	} else {
		leftSide = fmt.Sprintf("%s %s %s  %s", dot, num, projName, title)
	}
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

	// Header line: project name + session name (if any) + title ... right-aligned ctx% + time
	ctxPart := contextStyle(s.ContextPct).Render(fmt.Sprintf("%d%%", s.ContextPct))
	timePart := dimStyle.Render(formatDuration(s.LastActive))
	rightSide := ctxPart + " " + timePart
	rightWidth := lipgloss.Width(rightSide)

	projName := s.ProjectName
	projPart := detailValueStyle.Render(projName) + "  "
	headerLeft := projPart

	if s.SessionName != "" {
		sessNamePart := lipgloss.NewStyle().Foreground(lipgloss.Color("183")).Render(s.SessionName) + "  "
		headerLeft += sessNamePart
	}
	headerLeftWidth := lipgloss.Width(headerLeft)

	maxTitleWidth := contentWidth - headerLeftWidth - rightWidth - 2
	if maxTitleWidth < 10 {
		maxTitleWidth = 10
	}
	title := truncateToWidth(s.Title, maxTitleWidth)
	leftSide := headerLeft + detailValueStyle.Render(title)
	gap := contentWidth - lipgloss.Width(leftSide) - rightWidth
	if gap < 1 {
		gap = 1
	}
	headerLine := leftSide + strings.Repeat(" ", gap) + rightSide

	// Info line: dir/session-id + messages + size
	sizeStr := formatSize(s.FileSize)
	dirWithSession := s.ProjectDir + "/" + s.ID
	msgsPart := "  " + detailValueStyle.Render(fmt.Sprintf("%d", s.MsgCount)) + detailLabelStyle.Render(" msgs") + "  " + detailValueStyle.Render(sizeStr)
	msgsWidth := lipgloss.Width(msgsPart)
	maxDirWidth := contentWidth - msgsWidth
	if maxDirWidth < 10 {
		maxDirWidth = 10
	}
	dirWithSession = truncateToWidth(dirWithSession, maxDirWidth)
	infoLine := dimStyle.Render(dirWithSession) + msgsPart

	lines := []string{
		headerLine,
		infoLine,
		"",
		status + hidden,
	}

	// Activity / terminal content below status (full width)
	hasPaneCapture := false
	if snap, ok := m.paneContent[s.ID]; ok && snap.Content != "" {
		hasPaneCapture = true
	}

	if hasPaneCapture {
		lines = append(lines, "")
		snap := m.paneContent[s.ID]
		paneLines := strings.Split(snap.Content, "\n")
		maxLines := m.activityLines()
		if len(paneLines) > maxLines {
			paneLines = paneLines[len(paneLines)-maxLines:]
		}
		for i, pl := range paneLines {
			paneLines[i] = truncateToWidth(pl, contentWidth)
		}
		// Dim the content for inactive sessions
		if s.ActiveSource != types.SourceTmux {
			for i, pl := range paneLines {
				paneLines[i] = dimStyle.Render(pl)
			}
		}
		lines = append(lines, paneLines...)
	}

	// Activity entries — only show when no pane capture available (avoids redundancy)
	if !hasPaneCapture {
		entries := m.activities[s.ID]
		if len(entries) == 0 && s.FilePath != "" {
			entries = activity.TailFile(s.FilePath, m.activityLines())
		}
		if len(entries) > 0 {
			lines = append(lines, "")
			maxEntries := m.activityLines()
			if maxEntries > len(entries) {
				maxEntries = len(entries)
			}
			aStyle := activityStyle
			if s.ActiveSource == types.SourceTmux || s.ActiveSource == types.SourceProc {
				aStyle = activeActivityStyle
			}
			for i := 0; i < maxEntries; i++ {
				text := truncateToWidth(activity.FormatEntry(entries[i]), contentWidth)
				lines = append(lines, aStyle.Render(text))
			}
		}
	}

	// Clamp all lines to contentWidth to prevent lipgloss wrapping.
	// Uses MaxWidth which handles ANSI escape codes correctly.
	clampStyle := lipgloss.NewStyle().MaxWidth(contentWidth)
	for i, l := range lines {
		if lipgloss.Width(l) > contentWidth {
			lines[i] = clampStyle.Render(l)
		}
	}

	// Pad to fill the reserved height so the layout doesn't shift.
	// detailPaneLines() includes border(2), so inner target = detailPaneLines - 2.
	targetInner := m.detailPaneLines() - 2
	for len(lines) < targetInner {
		lines = append(lines, "")
	}
	if len(lines) > targetInner {
		lines = lines[:targetInner]
	}

	content := strings.Join(lines, "\n")
	styled := detailBorderStyle.Width(detailWidth).Render(content)

	return styled
}

func (m Model) renderProjects() string {
	gl := m.computeGridLayout()
	if gl == nil {
		return dimStyle.Render("  no projects")
	}

	gap := 2
	var lines []string
	for r := 0; r < gl.rows; r++ {
		var line strings.Builder
		line.WriteString("  ")
		for c := 0; c < gl.cols; c++ {
			idx := r*gl.cols + c
			if idx >= len(gl.names) {
				break
			}
			name := gl.names[idx]
			padded := name + strings.Repeat(" ", gl.colWidths[c]-len(name))
			if c < gl.cols-1 {
				padded += strings.Repeat(" ", gap)
			}

			if m.focus == FocusProjects && idx == m.projectIdx {
				line.WriteString(selectedProjectStyle.Render(padded))
			} else if m.filteredProj[idx].Hidden {
				line.WriteString(hiddenProjectStyle.Render(padded))
			} else {
				line.WriteString(normalProjectStyle.Render(padded))
			}
		}
		lines = append(lines, line.String())
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderFooter() string {
	var hints []string
	if m.focus == FocusSessions && len(m.filtered) > 0 {
		hints = append(hints, "enter switch/resume", "f follow")
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
		"  enter       Switch to or resume session (tmux)",
		"  f           Follow active session (split pane)",
		"  esc         Exit follow mode / clear filter",
		"  n           Jump to projects section",
		"  /           Toggle filter bar",
		"  tab         Switch: sessions ↔ projects",
		"  j/k ↑/↓     Navigate (↑↓←→ in projects)",
		"  gg/G        Jump to top/bottom",
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
		{"Project name length", fmt.Sprintf("%d", m.config.ProjectNameMax), false},
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

// truncateName truncates a project name to maxLen runes, appending "…" if truncated.
func truncateName(name string, maxLen int) string {
	runes := []rune(name)
	if len(runes) <= maxLen {
		return name
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

// truncateToWidth truncates a string to fit within maxWidth visual columns.
// Uses lipgloss.Width for accurate measurement (handles multi-byte UTF-8).
func truncateToWidth(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 {
		candidate := string(runes[:len(runes)-1])
		if lipgloss.Width(candidate)+1 <= maxWidth { // +1 for "…"
			return candidate + "…"
		}
		runes = runes[:len(runes)-1]
	}
	return "…"
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
