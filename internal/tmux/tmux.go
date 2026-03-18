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

// NewWindow creates a new tmux window and returns its window ID.
func NewWindow(name, dir string, cmdAndArgs []string) (string, error) {
	args := append([]string{"new-window", "-P", "-F", "#{window_id}", "-n", name, "-c", dir}, cmdAndArgs...)
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// SelectWindow focuses the given tmux window.
func SelectWindow(windowID string) error {
	return exec.Command("tmux", "select-window", "-t", windowID).Run()
}

// CapturePaneContent captures the last N lines of a tmux window's visible output.
// Returns raw terminal output with trailing empty lines trimmed.
// Content transformation (status bar stripping, task collapsing) is handled by
// capture.TransformPaneContent.
func CapturePaneContent(windowID string, lines int) (string, error) {
	if lines <= 0 {
		lines = 30
	}
	captureLines := lines + 15
	args := []string{"capture-pane", "-t", windowID, "-p", "-S", fmt.Sprintf("-%d", captureLines)}
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return "", err
	}
	content := string(out)
	content = strings.TrimRight(content, "\n")
	return content, nil
}

// PanePIDs returns a map of PID → window ID for all panes in the current tmux server.
func PanePIDs() (map[int]string, error) {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{window_id} #{pane_pid}").Output()
	if err != nil {
		return nil, err
	}
	result := make(map[int]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		pid := 0
		fmt.Sscanf(parts[1], "%d", &pid)
		if pid > 0 {
			result[pid] = parts[0]
		}
	}
	return result, nil
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
