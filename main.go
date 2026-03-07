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

	activeDirs := session.DetectActive()

	// Mark active sessions
	for i := range sessions {
		s := &sessions[i]
		encoded := "-" + filepath.ToSlash(s.ProjectDir)
		encoded = filepath.Clean(encoded)
		// Re-encode: the active detection uses path-to-encoded
		for dir := range activeDirs {
			_, absPath := session.DecodeProjectDir(dir)
			if absPath == s.ProjectDir {
				s.IsActive = true
				break
			}
		}
	}

	projects := project.DiscoverProjects(sessions, activeDirs, cfg)
	model := tui.New(sessions, projects, cfg)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
