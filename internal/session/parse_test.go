package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeProjectDir(t *testing.T) {
	tests := []struct {
		encoded  string
		wantName string
		wantPath string
	}{
		{"-home-mse-Projects-poe-proof", "poe-proof", "/home/mse/Projects/poe-proof"},
		{"-home-mse", "~", "/home/mse"},
		{"-home-mse--openclaw", ".openclaw", "/home/mse/.openclaw"},
		{"-home-mase-Projects-explorer", "explorer", "/home/mase/Projects/explorer"},
		{"-home-mase", "~", "/home/mase"},
		{"-home-mase--hermes", ".hermes", "/home/mase/.hermes"},
		{"-home-mse-Projects-ccs", "ccs", "/home/mse/Projects/ccs"},
		// macOS paths
		{"-Users-john-Projects-myapp", "myapp", "/Users/john/Projects/myapp"},
		{"-Users-john", "~", "/Users/john"},
		{"-Users-john--config", ".config", "/Users/john/.config"},
		{"-Users-john-Documents-work", "Documents-work", "/Users/john/Documents-work"},
	}

	for _, tt := range tests {
		t.Run(tt.encoded, func(t *testing.T) {
			name, path := DecodeProjectDir(tt.encoded)
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
		})
	}
}

func TestParseSession_Synthetic(t *testing.T) {
	// Create a temporary JSONL file with known content
	dir := t.TempDir()
	fpath := filepath.Join(dir, "test-session.jsonl")

	lines := []map[string]any{
		{
			"type":      "user",
			"sessionId": "abcdef12-3456-7890-abcd-ef1234567890",
			"isMeta":    true,
			"message": map[string]any{
				"role":    "user",
				"content": "<local-command-caveat>meta message</local-command-caveat>",
			},
		},
		{
			"type":      "user",
			"sessionId": "abcdef12-3456-7890-abcd-ef1234567890",
			"isMeta":    false,
			"message": map[string]any{
				"role":    "user",
				"content": "Hello, can you help me with something?",
			},
		},
		{
			"type":      "assistant",
			"sessionId": "abcdef12-3456-7890-abcd-ef1234567890",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "text", "text": "Sure!"}},
				"usage": map[string]any{
					"input_tokens":                 5000,
					"cache_creation_input_tokens":  10000,
					"cache_read_input_tokens":      15000,
					"output_tokens":                500,
				},
			},
		},
		{
			"type":      "user",
			"sessionId": "abcdef12-3456-7890-abcd-ef1234567890",
			"isMeta":    false,
			"message": map[string]any{
				"role":    "user",
				"content": "Thanks!",
			},
		},
		{
			"type":      "assistant",
			"sessionId": "abcdef12-3456-7890-abcd-ef1234567890",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "text", "text": "You're welcome!"}},
				"usage": map[string]any{
					"input_tokens":                 8000,
					"cache_creation_input_tokens":  12000,
					"cache_read_input_tokens":      20000,
					"output_tokens":                600,
				},
			},
		},
	}

	f, err := os.Create(fpath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		b, _ := json.Marshal(line)
		f.Write(b)
		f.Write([]byte("\n"))
	}
	f.Close()

	sess, err := ParseSession(fpath)
	if err != nil {
		t.Fatalf("ParseSession failed: %v", err)
	}

	// Session ID comes from the filename, not JSONL content
	if sess.ID != "test-session" {
		t.Errorf("ID = %q, want %q", sess.ID, "test-session")
	}
	if sess.ShortID != "test-ses" {
		t.Errorf("ShortID = %q, want %q", sess.ShortID, "test-ses")
	}
	// 3 user messages (1 meta + 2 real)
	if sess.MsgCount != 3 {
		t.Errorf("MsgCount = %d, want 3", sess.MsgCount)
	}
	// Title should be fallback from first non-meta user message
	if sess.Title != "Hello, can you help me with something?" {
		t.Errorf("Title = %q, want first non-meta user message", sess.Title)
	}
	// Context % = (8000+12000+20000)*100/200000 = 20
	if sess.ContextPct != 20 {
		t.Errorf("ContextPct = %d, want 20", sess.ContextPct)
	}
}

func TestParseSession_WithRename(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "renamed-session.jsonl")

	lines := []map[string]any{
		{
			"type":      "user",
			"sessionId": "rename1234-5678-9abc-def0-123456789abc",
			"isMeta":    false,
			"message": map[string]any{
				"role":    "user",
				"content": "Initial question about Go",
			},
		},
		{
			"type":      "assistant",
			"sessionId": "rename1234-5678-9abc-def0-123456789abc",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "text", "text": "Here's the answer"}},
				"usage": map[string]any{
					"input_tokens":                 1000,
					"cache_creation_input_tokens":  0,
					"cache_read_input_tokens":      0,
					"output_tokens":                100,
				},
			},
		},
		{
			"type":      "user",
			"sessionId": "rename1234-5678-9abc-def0-123456789abc",
			"isMeta":    false,
			"message": map[string]any{
				"role":    "user",
				"content": "<local-command-stdout>Session renamed to: my-cool-session</local-command-stdout>",
			},
		},
	}

	f, _ := os.Create(fpath)
	for _, line := range lines {
		b, _ := json.Marshal(line)
		f.Write(b)
		f.Write([]byte("\n"))
	}
	f.Close()

	sess, err := ParseSession(fpath)
	if err != nil {
		t.Fatalf("ParseSession failed: %v", err)
	}

	if sess.Title != "my-cool-session" {
		t.Errorf("Title = %q, want %q", sess.Title, "my-cool-session")
	}
}

func TestParseSession_Untitled(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "untitled.jsonl")

	lines := []map[string]any{
		{
			"type":      "user",
			"sessionId": "untitled0-0000-0000-0000-000000000000",
			"isMeta":    true,
			"message": map[string]any{
				"role":    "user",
				"content": "<local-command-caveat>meta only</local-command-caveat>",
			},
		},
	}

	f, _ := os.Create(fpath)
	for _, line := range lines {
		b, _ := json.Marshal(line)
		f.Write(b)
		f.Write([]byte("\n"))
	}
	f.Close()

	sess, err := ParseSession(fpath)
	if err != nil {
		t.Fatalf("ParseSession failed: %v", err)
	}

	if sess.Title != "(untitled)" {
		t.Errorf("Title = %q, want (untitled)", sess.Title)
	}
}

func TestParseSession_TeleportedSession(t *testing.T) {
	// Teleported sessions have a file-history-snapshot first (no sessionId),
	// then content with the original web session ID, then local content with
	// the filename-based session ID. The parser must use the filename, not content.
	dir := t.TempDir()
	localID := "63e426b0-544d-4b92-bc74-06fbaf477db6"
	fpath := filepath.Join(dir, localID+".jsonl")

	lines := []map[string]any{
		// First line: no sessionId (teleport artifact)
		{
			"type":      "file-history-snapshot",
			"messageId": "314bbc76-7228-458e-b40a-5e9b8812a68f",
		},
		// Teleported web content with the WRONG (web) session ID
		{
			"type":      "user",
			"sessionId": "cfb5dc3b-01d0-4b3f-92f7-7c70a5851371",
			"isMeta":    false,
			"message": map[string]any{
				"role":    "user",
				"content": "original web message",
			},
		},
		// Local content after teleport with the correct session ID
		{
			"type":      "user",
			"sessionId": localID,
			"isMeta":    false,
			"message": map[string]any{
				"role":    "user",
				"content": "local followup message",
			},
		},
	}

	f, _ := os.Create(fpath)
	for _, line := range lines {
		b, _ := json.Marshal(line)
		f.Write(b)
		f.Write([]byte("\n"))
	}
	f.Close()

	sess, err := ParseSession(fpath)
	if err != nil {
		t.Fatalf("ParseSession failed: %v", err)
	}

	// Must use filename-based ID, not the web session ID from content
	if sess.ID != localID {
		t.Errorf("ID = %q, want %q (filename-based)", sess.ID, localID)
	}
	if sess.Title != "original web message" {
		t.Errorf("Title = %q, want %q", sess.Title, "original web message")
	}
}

func TestDiscoverSessions_Synthetic(t *testing.T) {
	// Create a mock projects directory structure
	dir := t.TempDir()

	projDir := filepath.Join(dir, "-home-mse-Projects-testproj")
	os.MkdirAll(projDir, 0o755)

	subagentsDir := filepath.Join(projDir, "subagents")
	os.MkdirAll(subagentsDir, 0o755)

	// Create a session file large enough (>25KB)
	fpath := filepath.Join(projDir, "session1.jsonl")
	writeTestSession(t, fpath, "sess1111-2222-3333-4444-555566667777", 26*1024)

	// Create a small file that should be skipped
	smallPath := filepath.Join(projDir, "small.jsonl")
	writeTestSession(t, smallPath, "small000-0000-0000-0000-000000000000", 1024)

	// Create a subagent file that should be skipped
	subPath := filepath.Join(subagentsDir, "sub.jsonl")
	writeTestSession(t, subPath, "sub00000-0000-0000-0000-000000000000", 26*1024)

	sessions, err := DiscoverSessions(dir)
	if err != nil {
		t.Fatalf("DiscoverSessions failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}

	if sessions[0].ProjectName != "testproj" {
		t.Errorf("ProjectName = %q, want testproj", sessions[0].ProjectName)
	}
	if sessions[0].ProjectDir != "/home/mse/Projects/testproj" {
		t.Errorf("ProjectDir = %q, want /home/mse/Projects/testproj", sessions[0].ProjectDir)
	}
}

// writeTestSession creates a JSONL file with padding to reach the target size.
func writeTestSession(t *testing.T, fpath string, sessionID string, targetSize int) {
	t.Helper()

	f, err := os.Create(fpath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	written := 0

	// Write a user line
	userLine := map[string]any{
		"type":      "user",
		"sessionId": sessionID,
		"isMeta":    false,
		"message": map[string]any{
			"role":    "user",
			"content": "Test question",
		},
	}
	b, _ := json.Marshal(userLine)
	n, _ := f.Write(b)
	f.Write([]byte("\n"))
	written += n + 1

	// Write assistant lines with padding to reach target size
	for written < targetSize {
		padding := ""
		remaining := targetSize - written
		if remaining > 200 {
			padding = fmt.Sprintf("%0*d", remaining/2, 0)
		}
		assistLine := map[string]any{
			"type":      "assistant",
			"sessionId": sessionID,
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "text", "text": "Response " + padding}},
				"usage": map[string]any{
					"input_tokens":                 5000,
					"cache_creation_input_tokens":  3000,
					"cache_read_input_tokens":      2000,
					"output_tokens":                100,
				},
			},
		}
		b, _ = json.Marshal(assistLine)
		n, _ = f.Write(b)
		f.Write([]byte("\n"))
		written += n + 1
	}
}

func TestCleanTitle(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "x"
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "Hello world", "Hello world"},
		{"h1 heading", "# My Heading", "My Heading"},
		{"h2 heading", "## Sub heading", "Sub heading"},
		{"bold text", "**bold text**", "bold text"},
		{"bold with trailing", "**Goal:** Build something", "Goal: Build something"},
		{"multi-line", "first line\nsecond line", "first line"},
		{"list item", "- list item", "list item"},
		{"quoted text", "> quoted text", "quoted text"},
		{"html tags", "<b>hello</b>", "hello"},
		{"long string truncated to 80", long, long[:80]},
		{"real heading with em dash", "# PoE Hub Integration — Implementation Plan", "PoE Hub Integration — Implementation Plan"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanTitle(tt.input)
			if got != tt.want {
				t.Errorf("cleanTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDiscoverSessions_RealFiles(t *testing.T) {
	projectsDir := filepath.Join(os.Getenv("HOME"), ".claude", "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		t.Skip("no real projects dir found")
	}

	sessions, err := DiscoverSessions(projectsDir)
	if err != nil {
		t.Fatalf("DiscoverSessions on real data failed: %v", err)
	}

	t.Logf("Found %d sessions", len(sessions))
	for i, s := range sessions {
		if i >= 5 {
			break
		}
		t.Logf("  [%s] %s — %q (ctx: %d%%, msgs: %d, %s)",
			s.ShortID, s.ProjectName, s.Title, s.ContextPct, s.MsgCount,
			s.LastActive.Format("2006-01-02 15:04"))
	}

	// Verify sorted by LastActive descending
	for i := 1; i < len(sessions); i++ {
		if sessions[i].LastActive.After(sessions[i-1].LastActive) {
			t.Errorf("sessions not sorted: index %d (%s) is after index %d (%s)",
				i, sessions[i].LastActive, i-1, sessions[i-1].LastActive)
		}
	}
}
