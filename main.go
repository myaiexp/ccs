package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"ccs/internal/config"
	"ccs/internal/ipc"
	"ccs/internal/session"
	"ccs/internal/state"
	"ccs/internal/tmux"
	"ccs/internal/tui"
	"ccs/internal/watcher"
)

func main() {
	// Subcommand dispatch — these exit immediately, no TUI startup
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "launch":
			handleLaunch()
			return
		case "notify-exit":
			handleNotifyExit()
			return
		}
	}

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

	tracker := session.LoadTracker()
	sessions, err := session.LoadSessions(projectsDir, tracker)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering sessions: %v\n", err)
		os.Exit(1)
	}

	// Create file watcher for activity monitoring
	w, err := watcher.New(cfg.ActivityLines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create watcher: %v\n", err)
		w = nil
	}
	if w != nil {
		defer w.Close()
	}

	st := state.Load()

	projectsRoot := filepath.Join(home, "Projects")
	model := tui.New(sessions, cfg, tracker, st, w, projectsDir, projectsRoot)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func handleLaunch() {
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	project := fs.String("project", "", "project directory")
	resume := fs.String("resume", "", "session ID to resume")
	prompt := fs.String("prompt", "", "initial prompt")
	onDone := fs.String("on-done", "", "action when session completes")
	fs.Parse(os.Args[2:])

	// Resolve project to absolute path
	projectDir := *project
	if projectDir != "" {
		abs, err := filepath.Abs(projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving project path: %v\n", err)
			os.Exit(1)
		}
		projectDir = abs
	}

	req := ipc.LaunchRequest{
		ProjectDir: projectDir,
		ResumeID:   *resume,
		Prompt:     *prompt,
		OnDone:     *onDone,
	}

	resp, err := ipc.Launch(ipc.SocketPath(), req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ccs is not running — start ccs first\n")
		os.Exit(1)
	}

	if !resp.OK {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
		os.Exit(1)
	}

	if resp.SessionID != "" {
		fmt.Println(resp.SessionID)
	}
}

func handleNotifyExit() {
	fs := flag.NewFlagSet("notify-exit", flag.ExitOnError)
	window := fs.String("window", "", "tmux window ID")
	fs.Parse(os.Args[2:])

	notif := ipc.ExitNotification{
		WindowID: *window,
	}

	// Fire-and-forget — ignore errors (ccs may have already exited)
	ipc.NotifyExit(ipc.SocketPath(), notif)
}
