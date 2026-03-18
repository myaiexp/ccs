package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ccs/internal/activity"
	"ccs/internal/capture"
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

	// Search results mode
	if m.filtering && len(m.searchResults) > 0 {
		for i, r := range m.searchResults {
			isSelected := i == m.searchIdx
			sections = append(sections, m.renderSearchResult(r, isSelected))
		}
		sections = append(sections, m.renderFooter())
		content := lipgloss.JoinVertical(lipgloss.Left, sections...)
		bs := borderStyle
		if m.width > 0 {
			bs = bs.Width(m.width - 2)
		}
		return bs.Render(content)
	}

	active := m.activeSessions()
	openList := m.openSessions()
	nActive := len(active)

	// ACTIVE section
	if nActive > 0 {
		activeHeader := sectionStyle.Render("ACTIVE") + dimStyle.Render(fmt.Sprintf(" (%d)", nActive))
		sections = append(sections, activeHeader)

		for i, s := range active {
			globalIdx := i // active sessions are first in filtered list
			sections = append(sections, m.renderActiveRow(globalIdx, s))
		}
	}

	// OPEN section
	openHeader := sectionStyle.Render("OPEN") + dimStyle.Render(fmt.Sprintf(" (%d)", len(openList)))
	sections = append(sections, openHeader)

	if len(openList) == 0 && nActive == 0 {
		sections = append(sections, dimStyle.Render("  no sessions"))
	} else if len(openList) > 0 {
		start, end := m.scrollWindow()
		for i := start; i < end; i++ {
			s := openList[i]
			globalIdx := nActive + i
			isSelected := globalIdx == m.sessionIdx
			if isSelected {
				sections = append(sections, m.renderDetail(s))
			} else {
				sections = append(sections, m.renderOpenRow(globalIdx+1, s))
			}
		}
		if len(openList) > (end - start) {
			openIdx := m.sessionIdx - nActive
			if openIdx < 0 {
				openIdx = 0
			}
			indicator := dimStyle.Render(fmt.Sprintf("  ── %d/%d ──", openIdx+1, len(openList)))
			sections = append(sections, indicator)
		}
	}

	// Done/untracked section (when visible)
	if m.showDoneUntracked {
		var doneUntracked []types.Session
		for _, s := range m.filtered {
			if s.StateStatus == types.StatusDone || s.StateStatus == types.StatusUntracked {
				doneUntracked = append(doneUntracked, s)
			}
		}
		if len(doneUntracked) > 0 {
			for _, s := range doneUntracked {
				globalIdx := -1
				for gi, fs := range m.filtered {
					if fs.ID == s.ID {
						globalIdx = gi
						break
					}
				}
				badge := "·"
				if s.StateStatus == types.StatusDone {
					badge = "✓"
				}
				isSelected := globalIdx == m.sessionIdx
				sections = append(sections, m.renderDoneRow(badge, s, isSelected))
			}
		}
	}

	// Footer
	if m.errMsg != "" {
		errLine := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			MarginTop(1).
			Render(m.errMsg)
		sections = append(sections, errLine)
	} else if m.renaming {
		renameView := lipgloss.NewStyle().
			MarginTop(1).
			Render(m.renameInput.View())
		sections = append(sections, renameView)
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

	header := titleStyle.Render("ccs")
	sortIndicator := dimStyle.Render(fmt.Sprintf("  sort: %s %s", m.sortField, m.sortDir))
	header += sortIndicator
	sections = append(sections, header)

	// Compressed session list
	sessCount := dimStyle.Render(fmt.Sprintf(" (%d)", len(m.filtered)))
	sessHeader := sectionStyle.Render("SESSIONS") + sessCount
	sections = append(sections, sessHeader)

	topRows := (m.height * 40 / 100) - 4
	if topRows < 3 {
		topRows = 3
	}
	if topRows > 8 {
		topRows = 8
	}
	if topRows > len(m.filtered) {
		topRows = len(m.filtered)
	}

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
			sections = append(sections, m.renderOpenRow(i+1, m.filtered[i]))
		}
	}

	// Follow pane
	var followedSess *types.Session
	for _, s := range m.filtered {
		if s.ID == m.followID {
			sess := s
			followedSess = &sess
			break
		}
	}

	contentWidth := m.width - 6
	if contentWidth < 40 {
		contentWidth = 40
	}
	paneWidth := contentWidth - 2

	paneTitle := "Following: "
	if followedSess != nil {
		paneTitle += followedSess.ProjectName + " — " + m.displayName(*followedSess)
	} else {
		paneTitle += m.followID
	}
	if lipgloss.Width(paneTitle) > paneWidth {
		paneTitle = paneTitle[:paneWidth-1] + "…"
	}
	paneHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render(paneTitle)

	paneText := dimStyle.Render("Waiting for capture...")
	if snap, ok := m.paneContent[m.followID]; ok && snap.Content != "" {
		paneLines := strings.Split(snap.Content, "\n")
		availPaneRows := m.height - len(sections) - 6
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

	followFooter := footerStyle.Render("f unfollow  esc exit  enter switch  / search  ? help  q quit")
	sections = append(sections, followFooter)

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	bs := borderStyle
	if m.width > 0 {
		bs = bs.Width(m.width - 2)
	}

	return bs.Render(content)
}

// renderActiveRow renders an expanded active session (2-3 lines).
func (m Model) renderActiveRow(globalIdx int, s types.Session) string {
	isSelected := globalIdx == m.sessionIdx
	contentWidth := m.width - 4

	dot := activeDot
	numStr := fmt.Sprintf("%4d", globalIdx+1)
	num := numStyle.Render(numStr)

	projName := s.ProjectName
	if len(projName) > 20 {
		projName = projName[:19] + "…"
	}

	name := m.displayName(s)
	if len([]rune(name)) > 30 {
		name = string([]rune(name)[:29]) + "…"
	}

	// Status from pane capture
	status := ""
	if snap, ok := m.paneContent[s.ID]; ok {
		status = capture.DeriveStatus(snap)
	}
	if status != "" {
		if len([]rune(status)) > 40 {
			status = string([]rune(status)[:40])
		}
		status = statusStyle.Render(status)
	}

	ctxStr := contextStyle(s.ContextPct).Render(fmt.Sprintf("%d%%", s.ContextPct))
	timeStr := dimStyle.Render(formatDuration(s.LastActive))
	rightSide := ctxStr + " " + timeStr
	rightWidth := lipgloss.Width(rightSide)

	leftParts := fmt.Sprintf("%s %s %s  %s", dot, num, projName, name)
	if status != "" {
		leftParts += "  " + status
	}
	gap := contentWidth - lipgloss.Width(leftParts) - rightWidth
	if gap < 1 {
		gap = 1
	}
	headerLine := leftParts + strings.Repeat(" ", gap) + rightSide

	if isSelected {
		headerLine = activeSelectedStyle.Render(truncateToWidth(headerLine, contentWidth))
	}

	var lines []string
	lines = append(lines, headerLine)

	// 1-2 lines of pane capture below
	if snap, ok := m.paneContent[s.ID]; ok && snap.Content != "" {
		paneLines := strings.Split(snap.Content, "\n")
		n := 2
		if len(paneLines) < n {
			n = len(paneLines)
		}
		for _, pl := range paneLines[len(paneLines)-n:] {
			pl = truncateToWidth(strings.TrimSpace(pl), contentWidth-4)
			if pl != "" {
				lines = append(lines, "    "+dimStyle.Render(pl))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// renderOpenRow renders a compact session row for the open section.
func (m Model) renderOpenRow(visNum int, s types.Session) string {
	contentWidth := m.width - 4

	// Badge based on state
	badge := openBadge
	switch s.StateStatus {
	case types.StatusActive:
		badge = activeDot
	case types.StatusDone:
		badge = doneBadge
	case types.StatusUntracked:
		badge = untrackedBadge
	}

	numStr := fmt.Sprintf("%4d", visNum)
	num := numStyle.Render(numStr)

	projName := s.ProjectName
	if len(projName) > 20 {
		projName = projName[:19] + "…"
	}

	name := m.displayName(s)

	ctxStr := contextStyle(s.ContextPct).Render(fmt.Sprintf("%d%%", s.ContextPct))
	timeStr := dimStyle.Render(formatDuration(s.LastActive))
	rightSide := ctxStr + " " + timeStr
	rightWidth := lipgloss.Width(rightSide)

	leftFixed := 7 + lipgloss.Width(projName) + 2 // badge+space+num+space + proj + gap
	maxName := contentWidth - leftFixed - rightWidth - 2
	if maxName < 10 {
		maxName = 10
	}
	name = truncateToWidth(name, maxName)

	leftSide := fmt.Sprintf("%s %s %s  %s", badge, num, projName, name)
	gap := contentWidth - lipgloss.Width(leftSide) - rightWidth
	if gap < 1 {
		gap = 1
	}
	return leftSide + strings.Repeat(" ", gap) + rightSide
}

// renderDoneRow renders a dimmed done/untracked session row.
func (m Model) renderDoneRow(badge string, s types.Session, isSelected bool) string {
	contentWidth := m.width - 4

	badgeRendered := dimStyle.Render(badge)
	if badge == "✓" {
		badgeRendered = doneBadge
	}

	projName := s.ProjectName
	if len(projName) > 20 {
		projName = projName[:19] + "…"
	}

	name := m.displayName(s)
	timeStr := dimStyle.Render(formatDuration(s.LastActive))

	leftFixed := 6 + lipgloss.Width(projName) + 2
	maxName := contentWidth - leftFixed - lipgloss.Width(timeStr) - 2
	if maxName < 10 {
		maxName = 10
	}
	name = truncateToWidth(name, maxName)

	line := fmt.Sprintf(" %s  %s  %s", badgeRendered, dimStyle.Render(projName), dimStyle.Render(name))
	gap := contentWidth - lipgloss.Width(line) - lipgloss.Width(timeStr)
	if gap < 1 {
		gap = 1
	}
	line += strings.Repeat(" ", gap) + timeStr

	if isSelected {
		line = activeSelectedStyle.Render(truncateToWidth(line, contentWidth))
	}

	return line
}

func (m Model) renderDetail(s types.Session) string {
	detailWidth := m.width - 6
	contentWidth := detailWidth - 2
	if detailWidth < 40 {
		detailWidth = 40
	}
	if contentWidth < 38 {
		contentWidth = 38
	}

	// Status with badge
	var status string
	switch s.ActiveSource {
	case types.SourceTmux:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("● active (tmux)")
	case types.SourceProc:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("● active (external)")
	default:
		if s.StateStatus == types.StatusOpen {
			status = dimStyle.Render("○ open")
		} else if s.StateStatus == types.StatusDone {
			status = dimStyle.Render("✓ done")
		} else {
			status = dimStyle.Render("· untracked")
		}
	}

	ctxPart := contextStyle(s.ContextPct).Render(fmt.Sprintf("%d%%", s.ContextPct))
	timePart := dimStyle.Render(formatDuration(s.LastActive))
	rightSide := ctxPart + " " + timePart
	rightWidth := lipgloss.Width(rightSide)

	projPart := detailValueStyle.Render(s.ProjectName) + "  "
	headerLeft := projPart

	name := m.displayName(s)
	headerLeftWidth := lipgloss.Width(headerLeft)
	maxNameWidth := contentWidth - headerLeftWidth - rightWidth - 2
	if maxNameWidth < 10 {
		maxNameWidth = 10
	}
	nameStr := truncateToWidth(name, maxNameWidth)
	leftSide := headerLeft + detailValueStyle.Render(nameStr)
	gap := contentWidth - lipgloss.Width(leftSide) - rightWidth
	if gap < 1 {
		gap = 1
	}
	headerLine := leftSide + strings.Repeat(" ", gap) + rightSide

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

	lines := []string{headerLine, infoLine, "", status}

	// Activity / terminal content
	if snap, ok := m.paneContent[s.ID]; ok && snap.Content != "" {
		lines = append(lines, "")
		paneLines := strings.Split(snap.Content, "\n")
		maxLines := m.activityLines()
		if len(paneLines) > maxLines {
			paneLines = paneLines[len(paneLines)-maxLines:]
		}
		for i, pl := range paneLines {
			paneLines[i] = truncateToWidth(pl, contentWidth)
		}
		if s.ActiveSource != types.SourceTmux {
			for i, pl := range paneLines {
				paneLines[i] = dimStyle.Render(pl)
			}
		}
		lines = append(lines, paneLines...)
	} else {
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

	clampStyle := lipgloss.NewStyle().MaxWidth(contentWidth)
	for i, l := range lines {
		if lipgloss.Width(l) > contentWidth {
			lines[i] = clampStyle.Render(l)
		}
	}

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

func (m Model) renderFooter() string {
	var hints []string
	if len(m.filtered) > 0 {
		sess := m.filtered[m.sessionIdx]
		if sess.StateStatus == types.StatusActive {
			hints = append(hints, "enter switch", "f follow")
		} else {
			hints = append(hints, "enter resume")
		}
	}

	doneN := m.doneCount()
	left := ""
	if doneN > 0 {
		left = dimStyle.Render(fmt.Sprintf("%d done", doneN))
		if m.showDoneUntracked {
			left += " · " + dimStyle.Render("h hide")
		} else {
			left += " · " + dimStyle.Render("h show")
		}
	}

	hints = append(hints, "c complete", "R rename", "/ search", "? help", "q quit")
	right := footerStyle.Render(strings.Join(hints, "  "))

	if left != "" {
		gap := m.width - 4 - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 2 {
			gap = 2
		}
		return lipgloss.NewStyle().MarginTop(1).Render(left + strings.Repeat(" ", gap) + right)
	}
	return footerStyle.Render(strings.Join(hints, "  "))
}

func (m Model) renderHelp() string {
	help := strings.Join([]string{
		titleStyle.Render("ccs — Claude Code Sessions"),
		"",
		"  1-9         Switch/resume session by number",
		"  enter       Active → switch, otherwise → resume",
		"  f           Follow active session (split pane)",
		"  esc         Exit follow mode / clear filter",
		"  /           Fuzzy search all sessions + project dirs",
		"  j/k ↑/↓     Navigate active + open list",
		"  gg/G        Jump to top/bottom",
		"  c           Mark session as done (complete)",
		"  o           Reopen a done session",
		"  R           Rename session",
		"  N           Re-trigger auto-naming",
		"  s           Cycle sort: time → ctx% → size → name",
		"  r           Reverse sort direction",
		"  d           Delete session (with confirm)",
		"  h           Toggle showing done/untracked",
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
		value string
		on    bool
	}
	items := []prefItem{
		{"Relative numbers (nvim-style)", "", m.config.RelativeNumbers},
		{"Activity lines", fmt.Sprintf("%d", m.config.ActivityLines), false},
		{"Auto-name lines", fmt.Sprintf("%d", m.config.AutoNameLines), false},
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
			indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Render("[" + item.value + "]")
		} else {
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

// renderSearchResult renders a single search result row with state badge.
func (m Model) renderSearchResult(r SearchResult, isSelected bool) string {
	contentWidth := m.width - 4

	var badge, name, projName, rightSide string

	if r.Session != nil {
		s := r.Session
		switch s.StateStatus {
		case types.StatusActive:
			badge = activeDot
		case types.StatusOpen:
			badge = openBadge
		case types.StatusDone:
			badge = doneBadge
		case types.StatusUntracked:
			badge = untrackedBadge
		}
		projName = s.ProjectName
		if len(projName) > 20 {
			projName = projName[:19] + "…"
		}
		name = m.displayName(*s)
		ctxStr := contextStyle(s.ContextPct).Render(fmt.Sprintf("%d%%", s.ContextPct))
		timeStr := dimStyle.Render(formatDuration(s.LastActive))
		rightSide = ctxStr + " " + timeStr
	} else {
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Render("▸")
		projName = r.DirName
		name = dimStyle.Render(r.DirPath)
		rightSide = dimStyle.Render("(new session)")
	}

	rightWidth := lipgloss.Width(rightSide)
	leftFixed := 3 + lipgloss.Width(projName) + 2 // badge+space + proj + gap
	maxName := contentWidth - leftFixed - rightWidth - 2
	if maxName < 10 {
		maxName = 10
	}
	name = truncateToWidth(name, maxName)

	leftSide := fmt.Sprintf(" %s %s  %s", badge, projName, name)
	gap := contentWidth - lipgloss.Width(leftSide) - rightWidth
	if gap < 1 {
		gap = 1
	}
	line := leftSide + strings.Repeat(" ", gap) + rightSide

	if isSelected {
		line = activeSelectedStyle.Render(truncateToWidth(line, contentWidth))
	}

	return line
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

func truncateToWidth(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 {
		candidate := string(runes[:len(runes)-1])
		if lipgloss.Width(candidate)+1 <= maxWidth {
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
	if h >= 24 {
		return fmt.Sprintf("%dd", h/24)
	}
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}
