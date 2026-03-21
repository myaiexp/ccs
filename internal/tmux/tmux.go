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

// MoveWindow moves a tmux window into the target session.
// Uses -d to avoid focus switch, trailing colon auto-assigns index.
func MoveWindow(windowID, targetSession string) error {
	return exec.Command("tmux", "move-window", "-s", windowID, "-t", targetSession+":", "-d").Run()
}

// SetWindowOption sets a user option on a tmux window.
func SetWindowOption(windowID, key, value string) error {
	return exec.Command("tmux", "set-option", "-w", "-t", windowID, "@"+key, value).Run()
}

// GetWindowOption gets a user option from a tmux window. Returns "" if unset.
func GetWindowOption(windowID, key string) string {
	out, err := exec.Command("tmux", "show-option", "-wv", "-t", windowID, "@"+key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RenameWindow changes a tmux window's display name.
func RenameWindow(windowID, name string) error {
	return exec.Command("tmux", "rename-window", "-t", windowID, name).Run()
}

// SetStatusFormat sets the tmux status line format for the current session.
// lineIndex: 0 or 1 (for two-line status). Uses session-scoped option.
func SetStatusFormat(lineIndex int, format string) error {
	opt := fmt.Sprintf("status-format[%d]", lineIndex)
	return exec.Command("tmux", "set-option", "-s", opt, format).Run()
}

// UnsetStatusFormat removes session-scoped status format overrides.
func UnsetStatusFormat() error {
	_ = exec.Command("tmux", "set-option", "-su", "status-format[0]").Run()
	_ = exec.Command("tmux", "set-option", "-su", "status-format[1]").Run()
	return nil
}

// SetStatusLines sets the number of status lines (1 or 2). Session-scoped.
func SetStatusLines(count int) error {
	return exec.Command("tmux", "set-option", "-s", "status", fmt.Sprintf("%d", count)).Run()
}

// ListKeys returns current keybindings for the given key table.
func ListKeys(table string) ([]string, error) {
	out, err := exec.Command("tmux", "list-keys", "-T", table).Output()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}

// BindKey registers a tmux keybinding in the given table.
// The command string is passed as-is (may contain if-shell, etc.).
func BindKey(table, key, command string) error {
	// Use sh -c to handle complex commands with if-shell
	args := []string{"bind-key", "-T", table, key}
	args = append(args, strings.Fields(command)...)
	return exec.Command("tmux", args...).Run()
}

// UnbindKey removes a tmux keybinding.
func UnbindKey(table, key string) error {
	return exec.Command("tmux", "unbind-key", "-T", table, key).Run()
}

// SetHook registers a window-scoped tmux hook.
func SetHook(windowID, hookName, command string) error {
	return exec.Command("tmux", "set-hook", "-w", "-t", windowID, hookName, command).Run()
}

// RemoveHook removes a window-scoped tmux hook.
func RemoveHook(windowID, hookName string) error {
	return exec.Command("tmux", "set-hook", "-uw", "-t", windowID, hookName).Run()
}

// CurrentWindowID returns the window ID of the currently focused window.
// Uses list-windows to find the active window.
func CurrentWindowID() (string, error) {
	out, err := exec.Command("tmux", "list-windows", "-F", "#{window_active} #{window_id}").Output()
	if err != nil {
		return "", err
	}
	return parseActiveWindowID(string(out))
}

// parseActiveWindowID extracts the active window ID from list-windows output.
func parseActiveWindowID(output string) (string, error) {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[0] == "1" {
			return parts[1], nil
		}
	}
	return "", fmt.Errorf("no active window found")
}

// AllPanesBySession returns pane PIDs grouped by session name and window ID.
// Used by ScanAndAdopt to find Claude processes outside the ccs session.
func AllPanesBySession() (map[string]map[string]int, error) {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{session_name} #{window_id} #{pane_pid}").Output()
	if err != nil {
		return nil, err
	}
	return parseAllPanes(string(out))
}

// parseAllPanes parses list-panes output into session → windowID → PID map.
func parseAllPanes(output string) (map[string]map[string]int, error) {
	result := make(map[string]map[string]int)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 3 {
			continue
		}
		sessName := parts[0]
		windowID := parts[1]
		pid := 0
		fmt.Sscanf(parts[2], "%d", &pid)
		if pid <= 0 {
			continue
		}
		if result[sessName] == nil {
			result[sessName] = make(map[string]int)
		}
		result[sessName][windowID] = pid
	}
	return result, nil
}

// SessionWindows returns all window IDs in the given tmux session.
func SessionWindows(sessionName string) ([]string, error) {
	out, err := exec.Command("tmux", "list-windows", "-t", sessionName, "-F", "#{window_id}").Output()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}

// CurrentSessionName returns the name of the current tmux session.
func CurrentSessionName() (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
