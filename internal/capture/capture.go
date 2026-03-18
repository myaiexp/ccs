package capture

import (
	"regexp"
	"strings"
	"time"

	"ccs/internal/tmux"
)

// Compiled patterns for DeriveStatus — initialized once.
var (
	waitingPromptRe  = regexp.MustCompile(`^❯|[$] ?$`)
	permissionRe     = regexp.MustCompile(`(?i)\b(Allow|Deny)\b.*\b(Allow|Deny)\b|\bpermission\b`)
	errorDetailRe    = regexp.MustCompile(`(?:Error:|error:)\s*(.+)`)
	failRe           = regexp.MustCompile(`\bFAIL\b`)
	spinnerLineRe    = regexp.MustCompile(`[✻*]|^[─━]+$`)
)

// PaneSnapshot holds the captured output of a tmux pane.
type PaneSnapshot struct {
	SessionID  string
	WindowID   string
	Content    string
	CapturedAt time.Time
}

// CapturePane captures the last N lines of a tmux window's visible output.
// Returns a PaneSnapshot or error if the window doesn't exist.
func CapturePane(sessionID, windowID string, lines int) (PaneSnapshot, error) {
	content, err := tmux.CapturePaneContent(windowID, lines)
	if err != nil {
		return PaneSnapshot{}, err
	}
	return PaneSnapshot{
		SessionID:  sessionID,
		WindowID:   windowID,
		Content:    content,
		CapturedAt: time.Now(),
	}, nil
}

// DeriveStatus analyzes pane capture content to determine session attention state.
// Returns a human-readable status string. Empty string for empty snapshots.
func DeriveStatus(snap PaneSnapshot) string {
	content := strings.TrimRight(snap.Content, "\n ")
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")

	// Bottom line checks
	bottom := lines[len(lines)-1]

	// 1. Waiting for input: shell prompt at bottom
	if waitingPromptRe.MatchString(bottom) {
		return "Waiting for input"
	}

	// 2. Permission prompt: recent lines with Allow/Deny patterns
	recentStart := len(lines) - 10
	if recentStart < 0 {
		recentStart = 0
	}
	for i := len(lines) - 1; i >= recentStart; i-- {
		if permissionRe.MatchString(lines[i]) {
			return "Permission prompt"
		}
	}

	// 3. Spinner/thinking: bottom line has spinner characters
	if spinnerLineRe.MatchString(bottom) {
		return "Thinking..."
	}

	// 4. Error: recent lines with error patterns
	for i := len(lines) - 1; i >= recentStart; i-- {
		line := lines[i]
		if m := errorDetailRe.FindStringSubmatch(line); m != nil {
			detail := strings.TrimSpace(m[1])
			if len([]rune(detail)) > 50 {
				detail = string([]rune(detail)[:50])
			}
			return "Error: " + detail
		}
		if failRe.MatchString(line) {
			detail := strings.TrimSpace(line)
			if len([]rune(detail)) > 50 {
				detail = string([]rune(detail)[:50])
			}
			return "Error: " + detail
		}
	}

	// 5. Fallback: last non-empty non-spinner line
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" && !spinnerLineRe.MatchString(line) {
			if len([]rune(line)) > 60 {
				line = string([]rune(line)[:60])
			}
			return line
		}
	}

	return ""
}
