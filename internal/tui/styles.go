package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Border for the whole view
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	// Title
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99"))

	// Section headers
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("245")).
			MarginTop(1)

	// Active session dot
	activeDot = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")).
			Render("●")

	// Inactive session dot
	inactiveDot = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("○")

	// External session dot (detected via /proc, not launched from ccs)
	externalDot = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Render("●")

	// Activity summary style (dim italic for inactive sessions)
	activityStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true)

	// Activity style for active sessions (brighter, still italic)
	activeActivityStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Italic(true)

	// Session number
	numStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	// Selection cursor arrow
	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")).
			Bold(true)

	// Project name in session list
	projectStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75"))

	// Context percentage colors
	contextGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	contextYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	contextRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	// Time/duration
	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	// Footer
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			MarginTop(1)

	// Selected project in grid
	selectedProjectStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("236"))

	// Normal project in grid
	normalProjectStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("75"))

	// Hidden project
	hiddenProjectStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Italic(true)

	// Session detail pane
	detailBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).
				Padding(0, 1)

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	detailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	// Help overlay
	helpStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(1, 2)

	// Follow mode pane
	followPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(0, 1).
			MarginTop(1)
)

func contextStyle(pct int) lipgloss.Style {
	if pct >= 80 {
		return contextRed
	}
	if pct >= 60 {
		return contextYellow
	}
	return contextGreen
}
