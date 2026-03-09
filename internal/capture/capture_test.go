package capture

import (
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
