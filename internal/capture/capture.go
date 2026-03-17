package capture

import (
	"time"

	"ccs/internal/tmux"
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
