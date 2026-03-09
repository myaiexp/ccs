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
// Strips status bar / HUD lines from the bottom (detected by lines of box-drawing chars).
func CapturePaneContent(windowID string, lines int) (string, error) {
	if lines <= 0 {
		lines = 30
	}
	// Capture extra lines to account for HUD that will be stripped
	captureLines := lines + 15
	args := []string{"capture-pane", "-t", windowID, "-p", "-S", fmt.Sprintf("-%d", captureLines)}
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return "", err
	}
	content := string(out)
	content = strings.TrimRight(content, "\n")
	content = stripStatusBar(content)
	content = stripTrailingNoise(content)
	return content, nil
}

// stripStatusBar removes status bar / HUD lines from the bottom of captured pane content.
// Scans the last 15 lines from the bottom for the topmost box-drawing separator line
// and strips everything from there downward (HUD content, prompt lines, etc).
func stripStatusBar(content string) string {
	lines := strings.Split(content, "\n")
	// Scan the last 15 lines for the topmost separator
	cutIdx := -1
	searchStart := len(lines) - 15
	if searchStart < 0 {
		searchStart = 0
	}
	for i := searchStart; i < len(lines); i++ {
		if isBoxDrawingLine(lines[i]) {
			if cutIdx == -1 {
				cutIdx = i // first (topmost) separator in the bottom region
			}
		}
	}
	if cutIdx > 0 {
		lines = lines[:cutIdx]
	}
	result := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	return result
}

// stripTrailingNoise removes trailing empty lines and spinner/status lines (✻ Thinking...)
// from the bottom of captured content. Returns whether a spinner was present (activity signal).
func stripTrailingNoise(content string) string {
	lines := strings.Split(content, "\n")
	// Strip from the bottom: empty lines and spinner lines
	for len(lines) > 0 {
		line := strings.TrimSpace(lines[len(lines)-1])
		if line == "" || isSpinnerLine(line) {
			lines = lines[:len(lines)-1]
		} else {
			break
		}
	}
	return strings.Join(lines, "\n")
}

// isSpinnerLine detects Claude's activity spinner lines (✻ Thinking..., ✻ Churned for Xm Xs, etc).
func isSpinnerLine(line string) bool {
	for _, r := range line {
		if r == ' ' {
			continue
		}
		// ✻ U+273B, ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏ braille spinner chars (U+2800-U+28FF)
		return r == '✻' || r == '⠋' || r == '⠙' || r == '⠹' || r == '⠸' ||
			r == '⠼' || r == '⠴' || r == '⠦' || r == '⠧' || r == '⠇' || r == '⠏'
	}
	return false
}

// isBoxDrawingLine returns true if a line is predominantly box-drawing characters,
// indicating a status bar separator.
func isBoxDrawingLine(line string) bool {
	if len(line) == 0 {
		return false
	}
	boxCount := 0
	total := 0
	for _, r := range line {
		if r == ' ' || r == '\t' {
			continue
		}
		total++
		// Box drawing chars: ─━═╌╍╎╏│┃ and related (U+2500-U+257F)
		// Also ▪▫● and similar decorative markers
		if r >= 0x2500 && r <= 0x257F {
			boxCount++
		}
	}
	// A separator line is mostly box drawing chars (>80% of non-space chars)
	return total > 10 && boxCount*100/total > 80
}

// PanePIDs returns a map of PID → window ID for all panes in the current tmux server.
func PanePIDs() map[int]string {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{window_id} #{pane_pid}").Output()
	if err != nil {
		return nil
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
	return result
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
