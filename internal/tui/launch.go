package tui

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"ccs/internal/types"
)

// ExecFinishedMsg is sent when a launched claude process exits.
type ExecFinishedMsg struct{ Err error }

// LaunchResume returns a tea.Cmd that runs claude --resume <id>
// in the correct project directory. TUI suspends and resumes on exit.
func LaunchResume(sess types.Session, flags []string) tea.Cmd {
	args := append(flags, "--resume", sess.ID)
	c := exec.Command("claude", args...)
	c.Dir = sess.ProjectDir
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return ExecFinishedMsg{Err: err}
	})
}

// LaunchNew returns a tea.Cmd that runs claude in the given
// project directory. TUI suspends and resumes on exit.
func LaunchNew(proj types.Project, flags []string) tea.Cmd {
	c := exec.Command("claude", flags...)
	c.Dir = proj.Dir
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return ExecFinishedMsg{Err: err}
	})
}
