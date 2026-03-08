package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"ccs/internal/config"
	"ccs/internal/project"
	"ccs/internal/session"
	"ccs/internal/tui"
	"ccs/internal/types"
	"ccs/internal/watcher"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	projectsDir := filepath.Join(home, ".claude", "projects")

	sessions, err := session.DiscoverSessions(projectsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering sessions: %v\n", err)
		os.Exit(1)
	}

	// Load tracker, prune dead PIDs, seed from /proc
	tracker := session.LoadTracker()
	tracker.Refresh()
	tracker.MatchNewSession(sessions)

	// Mark sessions as open based on tracker, with ActiveSource
	openIDs := tracker.OpenSessionIDs()
	tmuxWindows := tracker.TmuxWindowIDs()
	for i := range sessions {
		if openIDs[sessions[i].ID] {
			sessions[i].IsActive = true
			if _, hasTmux := tmuxWindows[sessions[i].ID]; hasTmux {
				sessions[i].ActiveSource = types.SourceTmux
			} else {
				sessions[i].ActiveSource = types.SourceProc
			}
		}
	}

	// Create file watcher for activity monitoring
	const defaultActivityLines = 5
	w, err := watcher.New(defaultActivityLines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create watcher: %v\n", err)
		w = nil
	}
	if w != nil {
		defer w.Close()
	}

	projects := project.DiscoverProjects(sessions, cfg)
	model := tui.New(sessions, projects, cfg, tracker, w)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
