package tabmgr

import (
	"fmt"
	"strings"

	"ccs/internal/tmux"
)

// RenderStatusLine updates the tmux status line.
// Line 1: tab bar with names and attention indicators.
// Line 2: attention summary (or collapses to 1 line if no attention needed).
func (m *Manager) RenderStatusLine() error {
	tabs := m.Tabs()

	currentID, err := tmux.CurrentWindowID()
	if err != nil {
		currentID = ""
	}

	// Get terminal width for formatting (default 200 if unavailable)
	maxWidth := 200

	line1 := FormatTabBar(tabs, currentID, maxWidth)
	line2 := FormatAttentionSummary(tabs)

	if line2 == "" {
		// Single line — no attention needed
		if err := tmux.SetStatusLines(1); err != nil {
			return err
		}
		return tmux.SetStatusFormat(0, line1)
	}

	// Two lines
	if err := tmux.SetStatusLines(2); err != nil {
		return err
	}
	if err := tmux.SetStatusFormat(0, line1); err != nil {
		return err
	}
	return tmux.SetStatusFormat(1, line2)
}

// FormatTabBar builds the tmux format string for line 1.
// Format: " ⌂  |  proj-a: name  | > proj-b: name  |  proj-c *  "
// - ⌂ = dashboard (always first, highlighted if currentWindowID is dashboard)
// - > = currently focused tab
// - * = attention needed (colored: #[fg=yellow] for waiting, #[fg=red] for error, #[fg=colour208] for permission)
// - maxWidth limits total width, excess tabs shown as "+N more"
func FormatTabBar(tabs []Tab, currentWindowID string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 200
	}

	var parts []string
	widthUsed := 0

	// Dashboard tab (always first)
	dashPart := formatDashboardTab(currentWindowID)
	dashWidth := plainWidth(dashPart)
	parts = append(parts, dashPart)
	widthUsed += dashWidth

	// Format each tab
	overflow := 0
	for _, tab := range tabs {
		part := formatTab(tab, currentWindowID)
		partWidth := plainWidth(part)

		// Check if adding this tab would exceed maxWidth
		// Reserve space for potential "+N more" suffix
		reservedWidth := 12 // " | +NN more"
		if widthUsed+3+partWidth+reservedWidth > maxWidth && len(parts) > 1 {
			overflow = len(tabs) - (len(parts) - 1) // -1 for dashboard
			break
		}

		parts = append(parts, part)
		widthUsed += 3 + partWidth // 3 for " | "
	}

	result := strings.Join(parts, " #[default]| ")

	if overflow > 0 {
		result += fmt.Sprintf(" #[default]| #[fg=colour245]+%d more#[default]", overflow)
	}

	return result
}

// FormatAttentionSummary builds line 2 content.
// Format: " proj-a waiting for input * proj-b error in tests "
// Returns empty string if no tabs need attention.
func FormatAttentionSummary(tabs []Tab) string {
	var items []string

	for _, tab := range tabs {
		if tab.Attention == "" {
			continue
		}

		name := tab.ProjectName
		if tab.DisplayName != "" {
			name = tab.DisplayName
		}

		var styled string
		switch tab.Attention {
		case "waiting":
			styled = fmt.Sprintf("#[fg=yellow]%s#[default] waiting for input", name)
		case "permission":
			styled = fmt.Sprintf("#[fg=colour208]%s#[default] needs permission", name)
		case "error":
			styled = fmt.Sprintf("#[fg=red]%s#[default] has error", name)
		default:
			styled = fmt.Sprintf("%s %s", name, tab.Attention)
		}

		items = append(items, styled)
	}

	if len(items) == 0 {
		return ""
	}

	return " " + strings.Join(items, " #[default]· ") + " "
}

// formatDashboardTab formats the dashboard entry.
func formatDashboardTab(currentWindowID string) string {
	// Dashboard is always window index 0 — we check if currentWindowID
	// looks like it could be the first window. Since we can't resolve
	// session:0 to a @-id here, we check if no tab matches (handled by caller).
	// For simplicity, we bold the dashboard if it appears to be focused.
	return " #[bold]⌂#[nobold] "
}

// formatTab formats a single tab entry for the tab bar.
func formatTab(tab Tab, currentWindowID string) string {
	isCurrent := tab.WindowID == currentWindowID

	name := tab.ProjectName
	if tab.DisplayName != "" {
		name = tab.ProjectName + ": " + tab.DisplayName
	}

	// Truncate name
	nameRunes := []rune(name)
	if len(nameRunes) > 25 {
		name = string(nameRunes[:25])
	}

	var result string
	if isCurrent {
		result = fmt.Sprintf("#[bold]▸ %s#[nobold]", name)
	} else {
		result = fmt.Sprintf(" %s", name)
	}

	// Add attention indicator
	if tab.Attention != "" {
		switch tab.Attention {
		case "waiting":
			result += " #[fg=yellow]●#[default]"
		case "permission":
			result += " #[fg=colour208]●#[default]"
		case "error":
			result += " #[fg=red]●#[default]"
		default:
			result += " ●"
		}
	}

	return result
}

// plainWidth estimates the visible width of a tmux format string
// by stripping #[...] style tags and counting the remaining runes.
func plainWidth(s string) int {
	// First strip all #[...] tags
	stripped := stripTmuxTags(s)
	// Count runes (each rune = 1 display column for estimation)
	return len([]rune(stripped))
}

// stripTmuxTags removes tmux style tags like #[fg=red] from a string.
func stripTmuxTags(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inTag := false
	for i := 0; i < len(s); i++ {
		if i+1 < len(s) && s[i] == '#' && s[i+1] == '[' {
			inTag = true
			i++ // skip '['
			continue
		}
		if inTag {
			if s[i] == ']' {
				inTag = false
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
