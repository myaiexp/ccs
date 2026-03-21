package tmux

import (
	"os/exec"
	"strings"
)

// SavedBindings holds the original keybindings captured before ccs overrides them.
type SavedBindings struct {
	Space string // original command for prefix+Space
	One   string // original command for prefix+1
	Two   string // original command for prefix+2
}

// CaptureBindings reads current prefix bindings for Space, 1, 2
// by parsing output of `tmux list-keys -T prefix`.
func CaptureBindings() (SavedBindings, error) {
	lines, err := ListKeys("prefix")
	if err != nil {
		return SavedBindings{}, err
	}
	return parseBindings(lines), nil
}

// parseBindings extracts Space, 1, 2 bindings from list-keys output lines.
// Each line is like: bind-key -T prefix 1 select-window -t :1
// The key is the 4th field, everything from the 5th field onward is the command.
func parseBindings(lines []string) SavedBindings {
	var saved SavedBindings
	for _, line := range lines {
		key, cmd := parseBindingLine(line)
		switch key {
		case "Space":
			saved.Space = cmd
		case "1":
			saved.One = cmd
		case "2":
			saved.Two = cmd
		}
	}
	return saved
}

// parseBindingLine extracts the key and command from a single list-keys line.
// Returns ("", "") if the line doesn't match the expected format.
func parseBindingLine(line string) (key, cmd string) {
	fields := strings.Fields(line)
	// Minimum: "bind-key -T prefix <key> <command...>"
	if len(fields) < 5 {
		return "", ""
	}
	// fields[0] = "bind-key", [1] = "-T", [2] = "prefix", [3] = key, [4:] = command
	if fields[0] != "bind-key" || fields[1] != "-T" || fields[2] != "prefix" {
		return "", ""
	}
	key = fields[3]
	cmd = strings.Join(fields[4:], " ")
	return key, cmd
}

// buildIfShellArgs constructs the exec args for a tmux bind-key with if-shell fallback.
// Returns the full args slice for exec.Command("tmux", args...).
func buildIfShellArgs(key, shellTest, ccsAction, fallbackAction string) []string {
	return []string{
		"bind-key", "-T", "prefix", key,
		"if-shell", shellTest, ccsAction, fallbackAction,
	}
}

// InstallCCSBindings registers ccs-scoped keybindings with if-shell fallbacks.
// Uses `tmux show -wv @ccs-managed 2>/dev/null` for conditional scoping.
// CCS actions:
//
//	Space → select-window -t :0 (dashboard)
//	1 → previous-window
//	2 → next-window
//
// Fallback actions: the captured original bindings.
// The if-shell test checks whether the current window has @ccs-managed set.
func InstallCCSBindings(saved SavedBindings) error {
	shellTest := "tmux show -wv @ccs-managed 2>/dev/null"

	bindings := []struct {
		key       string
		ccsAction string
		fallback  string
	}{
		{"Space", "select-window -t :0", saved.Space},
		{"1", "previous-window", saved.One},
		{"2", "next-window", saved.Two},
	}

	for _, b := range bindings {
		fallback := b.fallback
		if fallback == "" {
			// No original binding — use a no-op as fallback
			fallback = "display-message ''"
		}
		args := buildIfShellArgs(b.key, shellTest, b.ccsAction, fallback)
		if err := exec.Command("tmux", args...).Run(); err != nil {
			return err
		}
	}
	return nil
}

// RestoreBindings removes ccs bindings and restores originals.
// Re-binds the original commands for Space, 1, 2.
func RestoreBindings(saved SavedBindings) error {
	restorations := []struct {
		key string
		cmd string
	}{
		{"Space", saved.Space},
		{"1", saved.One},
		{"2", saved.Two},
	}

	for _, r := range restorations {
		if r.cmd != "" {
			// Restore original binding — use exec.Command directly
			// because BindKey splits by spaces which works fine for simple commands.
			if err := BindKey("prefix", r.key, r.cmd); err != nil {
				// Log error but continue restoring other bindings
				continue
			}
		} else {
			// No original binding existed — unbind to clean up
			_ = UnbindKey("prefix", r.key)
		}
	}
	return nil
}
