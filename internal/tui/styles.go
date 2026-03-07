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

	// Session number
	numStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	// Selected item
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("236"))

	// Normal item text
	normalStyle = lipgloss.NewStyle()

	// Project name
	projectStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75"))

	// Session title
	titleTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

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

	// Filter prompt
	filterPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("99"))

	// Help overlay
	helpStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(1, 2)
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
