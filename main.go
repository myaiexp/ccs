package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"ccs/internal/config"
	"ccs/internal/project"
	"ccs/internal/session"
	"ccs/internal/tmux"
	"ccs/internal/tui"
	"ccs/internal/watcher"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Bootstrap into tmux if not already inside
	if !tmux.InTmux() {
		if err := tmux.Bootstrap(cfg.TmuxSessionName); err != nil {
			fmt.Fprintf(os.Stderr, "Error bootstrapping tmux: %v\n", err)
			os.Exit(1)
		}
		// Bootstrap replaces the process — should never reach here
		return
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

	tracker.MarkActive(sessions)

	// Create file watcher for activity monitoring
	w, err := watcher.New(cfg.ActivityLines)
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
