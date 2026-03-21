package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"ccs/internal/config"
	"ccs/internal/ipc"
	"ccs/internal/session"
	"ccs/internal/state"
	"ccs/internal/tabmgr"
	"ccs/internal/tmux"
	"ccs/internal/tui"
	"ccs/internal/watcher"
)

func main() {
	// 1. Subcommand dispatch — these exit immediately, no TUI startup
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "launch":
			handleLaunch()
			return
		case "notify-exit":
			handleNotifyExit()
			return
		case "cleanup":
			handleCleanup()
			return
		}
	}

	// 2. Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 3. Bootstrap into tmux if not already inside
	if !tmux.InTmux() {
		if err := tmux.Bootstrap(cfg.TmuxSessionName); err != nil {
			fmt.Fprintf(os.Stderr, "Error bootstrapping tmux: %v\n", err)
			os.Exit(1)
		}
		// Bootstrap replaces the process — should never reach here
		return
	}

	// 4. Clean up any stale ccs state from a previous crash
	cleanupStaleCCSState()

	// 5. Set @ccs-managed on current (dashboard) window
	currentWin, _ := tmux.CurrentWindowID()
	if currentWin != "" {
		_ = tmux.SetWindowOption(currentWin, "ccs-managed", "1")
	}

	// 6. Start IPC server
	socketPath := ipc.SocketPath()
	_ = os.MkdirAll(filepath.Dir(socketPath), 0700)
	server, err := ipc.NewServer(socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting IPC server: %v\n", err)
		os.Exit(1)
	}

	// 7. Signal handler for cleanup
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 8. Capture and install keybindings
	savedBindings, _ := tmux.CaptureBindings()
	_ = tmux.InstallCCSBindings(savedBindings)

	// 9. Set up status line
	_ = tmux.SetStatusLines(2)

	// 10. Create tab manager
	sessName, _ := tmux.CurrentSessionName()
	tracker := session.LoadTracker()
	st := state.Load()
	manager := tabmgr.New(sessName, tracker, st, cfg.ClaudeFlags)

	// 11. Start IPC server goroutine (handler wired after tea.NewProgram below)
	go server.Serve()

	// 12. Load sessions, watcher
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	projectsDir := filepath.Join(home, ".claude", "projects")

	sessions, err := session.LoadSessions(projectsDir, tracker)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering sessions: %v\n", err)
		os.Exit(1)
	}

	w, err := watcher.New(cfg.ActivityLines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create watcher: %v\n", err)
		w = nil
	}
	if w != nil {
		defer w.Close()
	}

	// 13. Create TUI
	projectsRoot := filepath.Join(home, "Projects")
	model := tui.New(sessions, cfg, tracker, st, w, manager, projectsDir, projectsRoot)

	// 14. Run TUI
	p := tea.NewProgram(model, tea.WithAltScreen())

	// 15. Wire IPC handlers (after tea.NewProgram so we can inject messages)
	server.SetHandler(ipc.Handler{
		OnLaunch: func(req ipc.LaunchRequest) ipc.LaunchResponse {
			windowID, err := manager.Launch(req.ProjectDir, req.ResumeID, req.Prompt, req.OnDone)
			if err != nil {
				return ipc.LaunchResponse{OK: false, Error: err.Error()}
			}
			return ipc.LaunchResponse{OK: true, SessionID: windowID}
		},
		OnExit: func(notif ipc.ExitNotification) {
			manager.HandleExit(notif.WindowID)
			p.Send(tui.TabExitMsg{WindowID: notif.WindowID})
		},
	})

	// 16. Cleanup function — runs on signal, error exit, and normal exit
	cleanup := func() {
		_ = tmux.RestoreBindings(savedBindings)
		tmux.ClearSavedBindings() // Clean exit — remove persisted originals
		_ = tmux.UnsetStatusFormat()
		_ = tmux.SetStatusLines(1)
		// Clear @ccs-managed from all windows in session
		if windows, err := tmux.SessionWindows(sessName); err == nil {
			for _, wid := range windows {
				_ = tmux.SetWindowOption(wid, "ccs-managed", "")
			}
		}
		server.Close()
	}

	// Run signal handler in background
	go func() {
		<-sigCh
		cleanup()
		os.Exit(0)
	}()

	if _, err := p.Run(); err != nil {
		cleanup()
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	cleanup()
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

// handleCleanup is the `ccs cleanup` subcommand — manually restores tmux state
// after a ccs crash that left stale keybindings, status format, or @ccs-managed options.
func handleCleanup() {
	if !tmux.InTmux() {
		fmt.Fprintln(os.Stderr, "Not in tmux — nothing to clean up")
		return
	}
	cleanupStaleCCSState()
	fmt.Println("Cleaned up stale ccs state")
}

// cleanupStaleCCSState detects and removes leftover ccs modifications from tmux.
// Runs on startup (to recover from crashes) and via `ccs cleanup`.
func cleanupStaleCCSState() {
	// Remove any ccs if-shell keybindings — restore tmux defaults
	lines, err := tmux.ListKeys("prefix")
	if err == nil {
		for _, line := range lines {
			if strings.Contains(line, "@ccs-managed") {
				// This is a stale ccs binding — figure out which key and unbind
				for _, key := range []string{"Space", "1", "2"} {
					if strings.Contains(line, " "+key+" ") || strings.HasSuffix(line, " "+key) {
						// Restore tmux default
						switch key {
						case "Space":
							_ = tmux.BindKey("prefix", key, "next-layout")
						case "1":
							_ = tmux.BindKey("prefix", key, "select-window -t :1")
						case "2":
							_ = tmux.BindKey("prefix", key, "select-window -t :2")
						}
					}
				}
			}
		}
	}

	// Remove stale saved bindings file
	tmux.ClearSavedBindings()

	// Unset any custom status format and restore single-line status
	_ = tmux.UnsetStatusFormat()
	_ = tmux.SetStatusLines(1)

	// Clear @ccs-managed from all windows in current session
	sessName, err := tmux.CurrentSessionName()
	if err == nil {
		if windows, err := tmux.SessionWindows(sessName); err == nil {
			for _, wid := range windows {
				_ = tmux.SetWindowOption(wid, "ccs-managed", "")
			}
		}
	}

	// Remove stale IPC socket
	_ = os.Remove(ipc.SocketPath())
}
