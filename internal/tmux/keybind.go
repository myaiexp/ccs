package tmux

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SavedBindings holds the original keybindings captured before ccs overrides them.
type SavedBindings struct {
	Space string `json:"space"` // original command for prefix+Space
	One   string `json:"one"`   // original command for prefix+1
	Two   string `json:"two"`   // original command for prefix+2
}

func savedBindingsPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = filepath.Join(os.TempDir(), "ccs")
	}
	return filepath.Join(dir, "ccs", "original-bindings.json")
}

// CaptureBindings reads current prefix bindings for Space, 1, 2.
// If a saved bindings file exists from a previous (possibly crashed) run,
// loads from that instead of re-parsing tmux state (which may contain
// corrupted ccs if-shell bindings).
func CaptureBindings() (SavedBindings, error) {
	// Check for persisted originals from a previous run
	if saved, err := loadSavedBindings(); err == nil {
		return saved, nil
	}

	// Fresh capture from tmux
	lines, err := ListKeys("prefix")
	if err != nil {
		return SavedBindings{}, err
	}
	saved := parseBindings(lines)

	// Filter out any leftover ccs bindings (from a crash that didn't clean up)
	saved = filterCCSBindings(saved)

	// Persist for crash recovery
	_ = persistSavedBindings(saved)

	return saved, nil
}

// ClearSavedBindings removes the persisted bindings file.
// Called on clean exit after restoring originals.
func ClearSavedBindings() {
	_ = os.Remove(savedBindingsPath())
}

func loadSavedBindings() (SavedBindings, error) {
	data, err := os.ReadFile(savedBindingsPath())
	if err != nil {
		return SavedBindings{}, err
	}
	var saved SavedBindings
	if err := json.Unmarshal(data, &saved); err != nil {
		return SavedBindings{}, err
	}
	return saved, nil
}

func persistSavedBindings(saved SavedBindings) error {
	p := savedBindingsPath()
	_ = os.MkdirAll(filepath.Dir(p), 0700)
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

// filterCCSBindings clears any binding that is a leftover ccs if-shell binding.
func filterCCSBindings(saved SavedBindings) SavedBindings {
	if strings.Contains(saved.Space, "@ccs-managed") {
		saved.Space = ""
	}
	if strings.Contains(saved.One, "@ccs-managed") {
		saved.One = ""
	}
	if strings.Contains(saved.Two, "@ccs-managed") {
		saved.Two = ""
	}
	return saved
}

// parseBindings extracts Space, 1, 2 bindings from list-keys output lines.
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

// parseBindingLine extracts the key and raw command from a single list-keys line.
// Preserves the exact command text including quotes and escaping.
// Format: "bind-key    -T prefix <key>   <command...>"
func parseBindingLine(line string) (key, cmd string) {
	// Must be a bind-key line (not unbind-key, etc.)
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "bind-key") {
		return "", ""
	}

	// Find " prefix " to locate where the key starts
	idx := strings.Index(line, " prefix ")
	if idx < 0 {
		return "", ""
	}

	// Skip "prefix" and whitespace to find the key
	rest := strings.TrimLeft(line[idx+8:], " \t") // 8 = len(" prefix ")

	// Key is the next non-whitespace token
	spaceIdx := strings.IndexAny(rest, " \t")
	if spaceIdx < 0 {
		return rest, "" // key with no command
	}

	key = rest[:spaceIdx]
	cmd = strings.TrimLeft(rest[spaceIdx:], " \t")
	return key, cmd
}

// buildIfShellArgs constructs the exec args for a tmux bind-key with if-shell fallback.
func buildIfShellArgs(key, shellTest, ccsAction, fallbackAction string) []string {
	return []string{
		"bind-key", "-T", "prefix", key,
		"if-shell", shellTest, ccsAction, fallbackAction,
	}
}

// InstallCCSBindings registers ccs-scoped keybindings with if-shell fallbacks.
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
// Uses sh -c to properly handle complex commands with quotes/escaping.
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
			// Use sh -c to handle complex commands (escaped quotes, semicolons, etc.)
			fullCmd := "tmux bind-key -T prefix " + r.key + " " + r.cmd
			if err := exec.Command("sh", "-c", fullCmd).Run(); err != nil {
				continue
			}
		} else {
			_ = UnbindKey("prefix", r.key)
		}
	}
	return nil
}
