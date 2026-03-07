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

	// Auto-cleanup: remove tiny session files
	session.Cleanup(projectsDir)

	sessions, err := session.DiscoverSessions(projectsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering sessions: %v\n", err)
		os.Exit(1)
	}

	active := session.DetectActive()
	markActiveSessions(sessions, active)

	projects := project.DiscoverProjects(sessions, active, cfg)
	model := tui.New(sessions, projects, cfg)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// markActiveSessions marks sessions as active based on detected claude processes.
// If a specific session ID was found (via --resume), only that session is marked.
// If claude is running in a project dir without --resume, the most recently
// modified session in that project is marked as active.
func markActiveSessions(sessions []types.Session, active types.ActiveInfo) {
	// First pass: mark sessions with exact ID match
	matchedDirs := make(map[string]bool)
	for i := range sessions {
		if active.SessionIDs[sessions[i].ID] {
			sessions[i].IsActive = true
			// Find which project dir this session belongs to, mark it as handled
			for dir := range active.ProjectDirs {
				_, absPath := session.DecodeProjectDir(dir)
				if absPath == sessions[i].ProjectDir {
					matchedDirs[dir] = true
				}
			}
		}
	}

	// Second pass: for project dirs with active claude but no --resume match,
	// mark the most recently modified session in that project
	for dir := range active.ProjectDirs {
		if matchedDirs[dir] {
			continue
		}
		_, absPath := session.DecodeProjectDir(dir)
		// Sessions are sorted by LastActive desc, so first match = most recent
		for i := range sessions {
			if sessions[i].ProjectDir == absPath {
				sessions[i].IsActive = true
				break
			}
		}
	}
}
