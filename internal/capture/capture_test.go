package capture

import (
	"strings"
	"testing"
)

func TestPaneSnapshot_Fields(t *testing.T) {
	snap := PaneSnapshot{SessionID: "abc", WindowID: "@1", Content: "hello"}
	if snap.SessionID != "abc" {
		t.Errorf("SessionID = %q, want %q", snap.SessionID, "abc")
	}
	if snap.WindowID != "@1" {
		t.Errorf("WindowID = %q, want %q", snap.WindowID, "@1")
	}
	if snap.Content != "hello" {
		t.Errorf("Content = %q, want %q", snap.Content, "hello")
	}
}

func TestCapturePane_InvalidWindow(t *testing.T) {
	_, err := CapturePane("test-session", "@99999", 20)
	if err == nil {
		t.Error("expected error for invalid window, got nil")
	}
}

func TestDeriveStatus_Empty(t *testing.T) {
	snap := PaneSnapshot{Content: ""}
	if got := DeriveStatus(snap); got != "" {
		t.Errorf("empty snapshot: got %q, want empty", got)
	}

	snap.Content = "   \n\n  "
	if got := DeriveStatus(snap); got != "" {
		t.Errorf("whitespace-only: got %q, want empty", got)
	}
}

func TestDeriveStatus_WaitingPrompt(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"fish prompt", "some output\n❯"},
		{"fish prompt with space", "some output\n❯ "},
		{"bash prompt", "user@host:~/proj$ "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := PaneSnapshot{Content: tt.content}
			if got := DeriveStatus(snap); got != "Waiting for input" {
				t.Errorf("got %q, want %q", got, "Waiting for input")
			}
		})
	}
}

func TestDeriveStatus_Permission(t *testing.T) {
	content := "Claude wants to edit file.go\n  Allow  Deny\nPress enter to confirm"
	snap := PaneSnapshot{Content: content}
	if got := DeriveStatus(snap); got != "Permission prompt" {
		t.Errorf("got %q, want %q", got, "Permission prompt")
	}
}

func TestDeriveStatus_Thinking(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"spinner star", "Working on changes\n✻ Generating..."},
		{"separator line", "some output\n────────────────────"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := PaneSnapshot{Content: tt.content}
			if got := DeriveStatus(snap); got != "Thinking..." {
				t.Errorf("got %q, want %q", got, "Thinking...")
			}
		})
	}
}

func TestDeriveStatus_Error(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			"Error: prefix",
			"compiling...\nError: cannot find module foo",
			"Error: cannot find module foo",
		},
		{
			"FAIL keyword",
			"running tests\nFAIL ccs/internal/tui",
			"Error: FAIL ccs/internal/tui",
		},
		{
			"error: lowercase",
			"building...\nerror: undefined reference to main",
			"Error: undefined reference to main",
		},
		{
			"long error truncated",
			"Error: " + strings.Repeat("x", 100),
			"Error: " + strings.Repeat("x", 50),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := PaneSnapshot{Content: tt.content}
			got := DeriveStatus(snap)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeriveStatus_Fallback(t *testing.T) {
	snap := PaneSnapshot{Content: "Editing internal/auth.go\nWriting tests"}
	got := DeriveStatus(snap)
	if got != "Writing tests" {
		t.Errorf("got %q, want %q", got, "Writing tests")
	}
}

func TestDeriveStatus_FallbackTruncated(t *testing.T) {
	long := strings.Repeat("a", 80)
	snap := PaneSnapshot{Content: long}
	got := DeriveStatus(snap)
	if len([]rune(got)) > 60 {
		t.Errorf("fallback should truncate to 60 chars, got %d", len([]rune(got)))
	}
}

func TestDeriveStatus_WaitingOverridesError(t *testing.T) {
	// If there's an error above but prompt is at bottom, "Waiting" wins
	content := "Error: something broke\n❯"
	snap := PaneSnapshot{Content: content}
	if got := DeriveStatus(snap); got != "Waiting for input" {
		t.Errorf("got %q, want %q", got, "Waiting for input")
	}
}

func TestDeriveStatus_FallbackSkipsSpinnerLines(t *testing.T) {
	content := "Actual content line\n✻ spinner line"
	snap := PaneSnapshot{Content: content}
	// The bottom is a spinner → "Thinking..."
	if got := DeriveStatus(snap); got != "Thinking..." {
		t.Errorf("got %q, want %q", got, "Thinking...")
	}
}
