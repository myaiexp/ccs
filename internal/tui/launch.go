package tui

import (
	"io"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"ccs/internal/session"
	"ccs/internal/types"
)

// ExecFinishedMsg is sent when a launched claude process exits.
type ExecFinishedMsg struct{ Err error }

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
	args := append(flags, "--resume", sess.ID)
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
