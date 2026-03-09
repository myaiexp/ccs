package tui

import (
	"fmt"
	"io"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"ccs/internal/session"
	"ccs/internal/tmux"
	"ccs/internal/types"
)

// ExecFinishedMsg is sent when a launched claude process exits.
type ExecFinishedMsg struct{ Err error }

// TmuxLaunchDoneMsg is sent when a tmux window launch completes.
type TmuxLaunchDoneMsg struct{ Err error }

// TmuxSwitchDoneMsg is sent when a tmux window switch completes.
type TmuxSwitchDoneMsg struct{ Err error }

// trackedCmd wraps an exec.Cmd to capture the PID after Start
// and record it in the session tracker.
type trackedCmd struct {
	cmd       *exec.Cmd
	tracker   *session.Tracker
	sessionID string
	projDir   string
}

func (t *trackedCmd) Run() error {
	if err := t.cmd.Start(); err != nil {
		return err
	}
	// Record PID in tracker now that the process is running
	if t.tracker != nil {
		t.tracker.Track(t.sessionID, t.projDir, t.cmd.Process.Pid)
	}
	return t.cmd.Wait()
}

func (t *trackedCmd) SetStdin(r io.Reader) {
	if t.cmd.Stdin == nil {
		t.cmd.Stdin = r
	}
}

func (t *trackedCmd) SetStdout(w io.Writer) {
	if t.cmd.Stdout == nil {
		t.cmd.Stdout = w
	}
}

func (t *trackedCmd) SetStderr(w io.Writer) {
	if t.cmd.Stderr == nil {
		t.cmd.Stderr = w
	}
}

// LaunchResume returns a tea.Cmd that runs claude --resume <id>
// in the correct project directory. TUI suspends and resumes on exit.
func LaunchResume(sess types.Session, flags []string, tracker *session.Tracker) tea.Cmd {
	args := make([]string, len(flags), len(flags)+2)
	copy(args, flags)
	args = append(args, "--resume", sess.ID)
	c := exec.Command("claude", args...)
	c.Dir = sess.ProjectDir
	return tea.Exec(&trackedCmd{
		cmd:       c,
		tracker:   tracker,
		sessionID: sess.ID,
		projDir:   sess.ProjectDir,
	}, func(err error) tea.Msg {
		return ExecFinishedMsg{Err: err}
	})
}

// LaunchNew returns a tea.Cmd that runs claude in the given
// project directory. TUI suspends and resumes on exit.
func LaunchNew(proj types.Project, flags []string, tracker *session.Tracker) tea.Cmd {
	c := exec.Command("claude", flags...)
	c.Dir = proj.Dir
	return tea.Exec(&trackedCmd{
		cmd:     c,
		tracker: tracker,
		projDir: proj.Dir,
	}, func(err error) tea.Msg {
		return ExecFinishedMsg{Err: err}
	})
}

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

