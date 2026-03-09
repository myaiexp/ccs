package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// InTmux returns true if the current process is running inside a tmux session.
func InTmux() bool {
	return os.Getenv("TMUX") != ""
}

// Bootstrap replaces the current process with a new tmux session running ccs.
// On success this function never returns.
func Bootstrap(sessionName string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	return syscall.Exec(tmuxPath, []string{"tmux", "new-session", "-s", sessionName, "ccs"}, os.Environ())
}

// newWindowArgs builds the argument slice for a tmux new-window command.
func newWindowArgs(name, dir string, cmdAndArgs []string) []string {
	args := []string{"new-window", "-P", "-F", "#{window_id}", "-n", name, "-c", dir}
	args = append(args, cmdAndArgs...)
	return args
}

// NewWindow creates a new tmux window and returns its window ID.
func NewWindow(name, dir string, cmdAndArgs []string) (string, error) {
	args := newWindowArgs(name, dir, cmdAndArgs)
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// selectWindowArgs builds the argument slice for a tmux select-window command.
func selectWindowArgs(windowID string) []string {
	return []string{"select-window", "-t", windowID}
}

// SelectWindow focuses the given tmux window.
func SelectWindow(windowID string) error {
	args := selectWindowArgs(windowID)
	return exec.Command("tmux", args...).Run()
}

// CapturePaneContent captures the last N lines of a tmux window's visible output.
// Returns raw terminal output with trailing empty lines trimmed.
func CapturePaneContent(windowID string, lines int) (string, error) {
	if lines <= 0 {
		lines = 30
	}
	args := []string{"capture-pane", "-t", windowID, "-p", "-S", fmt.Sprintf("-%d", lines)}
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return "", err
	}
	content := string(out)
	// Trim trailing empty lines
	content = strings.TrimRight(content, "\n")
	return content, nil
}

// WindowExists checks whether a tmux window with the given ID exists.
func WindowExists(windowID string) bool {
	out, err := exec.Command("tmux", "list-windows", "-F", "#{window_id}").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == windowID {
			return true
		}
	}
	return false
}
