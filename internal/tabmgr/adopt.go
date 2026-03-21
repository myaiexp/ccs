package tabmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ccs/internal/tmux"
)

// ScanAndAdopt finds Claude processes in tmux windows outside the ccs session
// and moves them in via tmux.MoveWindow.
// Steps:
// 1. Call tmux.AllPanesBySession() to get all panes grouped by session
// 2. Filter for panes NOT in m.sessionName
// 3. Check if the pane PID is a claude process (check /proc/<pid>/cmdline for "claude")
// 4. For each match: tmux.MoveWindow, set @ccs-managed, set pane-exited hook, register as tab
// Returns adopted window IDs.
func (m *Manager) ScanAndAdopt() ([]string, error) {
	allPanes, err := tmux.AllPanesBySession()
	if err != nil {
		return nil, fmt.Errorf("listing panes: %w", err)
	}

	var adopted []string

	for sessName, windows := range allPanes {
		// Skip panes in the ccs session
		if sessName == m.sessionName {
			continue
		}

		for windowID, pid := range windows {
			if !isClaudeProcess(pid) {
				continue
			}

			// Get the working directory of the process
			projectDir := readProcessCwd(pid)
			projectName := filepath.Base(projectDir)

			// Move the window into the ccs session
			if err := tmux.MoveWindow(windowID, m.sessionName); err != nil {
				continue
			}

			// Set @ccs-managed option
			_ = tmux.SetWindowOption(windowID, "ccs-managed", "1")

			// Set pane-exited hook
			hookCmd := fmt.Sprintf("run-shell 'ccs notify-exit --window %s'", windowID)
			_ = tmux.SetHook(windowID, "pane-exited", hookCmd)

			tab := Tab{
				WindowID:    windowID,
				ProjectDir:  projectDir,
				ProjectName: projectName,
			}

			m.mu.Lock()
			m.tabs = append(m.tabs, tab)
			m.mu.Unlock()

			adopted = append(adopted, windowID)
		}
	}

	return adopted, nil
}

// isClaudeProcess checks if a PID is running a claude process by reading /proc/<pid>/cmdline.
func isClaudeProcess(pid int) bool {
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	// cmdline is null-separated; check if any arg contains "claude"
	return strings.Contains(string(cmdline), "claude")
}

// readProcessCwd returns the current working directory of a process via /proc/<pid>/cwd.
func readProcessCwd(pid int) string {
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return ""
	}
	return cwd
}
