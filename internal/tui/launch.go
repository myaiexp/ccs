package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"ccs/internal/session"
	"ccs/internal/tmux"
	"ccs/internal/types"
)

// TmuxLaunchDoneMsg is sent when a tmux window launch completes.
type TmuxLaunchDoneMsg struct{ Err error }

// TmuxSwitchDoneMsg is sent when a tmux window switch completes.
type TmuxSwitchDoneMsg struct{ Err error }

// tmuxWindowName builds a window name like "proj: title", truncated to 30 chars.
func tmuxWindowName(proj, title string) string {
	name := fmt.Sprintf("%s: %s", proj, title)
	if len(name) > 30 {
		name = name[:30]
	}
	return name
}

// TmuxLaunchResume creates a new tmux window to resume a session.
// The TUI stays visible (no tea.Exec). Returns RefreshMsg on completion.
func TmuxLaunchResume(sess types.Session, flags []string, tracker *session.Tracker) tea.Cmd {
	return func() tea.Msg {
		name := tmuxWindowName(sess.ProjectName, sess.Title)
		resumeArgs := make([]string, 0, len(flags)+3)
		resumeArgs = append(resumeArgs, "claude")
		resumeArgs = append(resumeArgs, flags...)
		resumeArgs = append(resumeArgs, "--resume", sess.ID)
		cmd := resumeArgs
		windowID, err := tmux.NewWindow(name, sess.ProjectDir, cmd)
		if err != nil {
			return TmuxLaunchDoneMsg{Err: err}
		}
		tracker.SetTmuxWindow(sess.ID, windowID)
		return TmuxLaunchDoneMsg{Err: nil}
	}
}

// TmuxLaunchNew creates a new tmux window for a new session in the given project.
func TmuxLaunchNew(proj types.Project, flags []string, tracker *session.Tracker) tea.Cmd {
	return func() tea.Msg {
		name := proj.Name
		if len(name) > 30 {
			name = name[:30]
		}
		cmd := make([]string, 0, len(flags)+1)
		cmd = append(cmd, "claude")
		cmd = append(cmd, flags...)
		_, err := tmux.NewWindow(name, proj.Dir, cmd)
		if err != nil {
			return TmuxLaunchDoneMsg{Err: err}
		}
		return TmuxLaunchDoneMsg{Err: nil}
	}
}

// TmuxSwitch focuses an existing tmux window.
func TmuxSwitch(windowID string) tea.Cmd {
	return func() tea.Msg {
		err := tmux.SelectWindow(windowID)
		return TmuxSwitchDoneMsg{Err: err}
	}
}

