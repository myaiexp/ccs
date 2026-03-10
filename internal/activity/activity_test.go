package activity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractFromLine_ToolUse(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/home/user/project/model.go","old_string":"foo","new_string":"bar"}}]},"timestamp":"2025-01-15T10:30:00Z"}`)

	entries := ExtractFromLine(line)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Type != "tool_use" {
		t.Errorf("expected type tool_use, got %q", e.Type)
	}
	if e.Tool != "Edit" {
		t.Errorf("expected tool Edit, got %q", e.Tool)
	}
	if e.Summary != "Edit: model.go" {
		t.Errorf("expected summary 'Edit: model.go', got %q", e.Summary)
	}
	expected := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	if !e.Timestamp.Equal(expected) {
		t.Errorf("expected timestamp %v, got %v", expected, e.Timestamp)
	}
}

func TestExtractFromLine_Text(t *testing.T) {
	longText := strings.Repeat("abcdefghij", 10) // 100 chars
	line := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + longText + `"}]}}`)

	entries := ExtractFromLine(line)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Type != "text" {
		t.Errorf("expected type text, got %q", e.Type)
	}
	if e.Tool != "" {
		t.Errorf("expected empty tool, got %q", e.Tool)
	}
	// Should preserve the full first line (up to 200 chars); display layer truncates to fit
	if len(e.Summary) > 200 {
		t.Errorf("expected summary <= 200 chars, got %d: %q", len(e.Summary), e.Summary)
	}
	if e.Summary != longText {
		t.Errorf("expected full text preserved, got %q", e.Summary)
	}
	if !e.Timestamp.IsZero() {
		t.Errorf("expected zero timestamp, got %v", e.Timestamp)
	}
}

func TestExtractFromLine_IgnoresUserMessages(t *testing.T) {
	line := []byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`)

	entries := ExtractFromLine(line)
	if entries != nil {
		t.Errorf("expected nil for user message, got %v", entries)
	}
}

func TestExtractFromLine_IgnoresResultMessages(t *testing.T) {
	line := []byte(`{"type":"result","result":"some output"}`)

	entries := ExtractFromLine(line)
	if entries != nil {
		t.Errorf("expected nil for result message, got %v", entries)
	}
}

func TestExtractFromLine_MultipleTools(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/foo.go"}},{"type":"tool_use","name":"Bash","input":{"command":"go test ./..."}}]}}`)

	entries := ExtractFromLine(line)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Tool != "Read" {
		t.Errorf("first entry: expected tool Read, got %q", entries[0].Tool)
	}
	if entries[0].Summary != "Read: foo.go" {
		t.Errorf("first entry: expected summary 'Read: foo.go', got %q", entries[0].Summary)
	}

	if entries[1].Tool != "Bash" {
		t.Errorf("second entry: expected tool Bash, got %q", entries[1].Tool)
	}
	if entries[1].Summary != "Bash: go test ./..." {
		t.Errorf("second entry: expected summary 'Bash: go test ./...', got %q", entries[1].Summary)
	}
}

func TestExtractFromLine_MalformedJSON(t *testing.T) {
	entries := ExtractFromLine([]byte(`{invalid json`))
	if entries != nil {
		t.Errorf("expected nil for malformed JSON, got %v", entries)
	}
}

func TestExtractFromLine_EmptyLine(t *testing.T) {
	entries := ExtractFromLine([]byte(""))
	if entries != nil {
		t.Errorf("expected nil for empty line, got %v", entries)
	}
}

func TestExtractFromLine_GrepTool(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Grep","input":{"pattern":"func.*Test"}}]}}`)

	entries := ExtractFromLine(line)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Summary != "Grep: func.*Test" {
		t.Errorf("expected 'Grep: func.*Test', got %q", entries[0].Summary)
	}
}

func TestExtractFromLine_GlobTool(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Glob","input":{"pattern":"**/*.go"}}]}}`)

	entries := ExtractFromLine(line)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Summary != "Glob: **/*.go" {
		t.Errorf("expected 'Glob: **/*.go', got %q", entries[0].Summary)
	}
}

func TestExtractFromLine_WriteTool(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Write","input":{"file_path":"/tmp/output.txt"}}]}}`)

	entries := ExtractFromLine(line)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Summary != "Write: output.txt" {
		t.Errorf("expected 'Write: output.txt', got %q", entries[0].Summary)
	}
}

func TestExtractFromLine_UnknownTool(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"CustomTool","input":{}}]}}`)

	entries := ExtractFromLine(line)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Summary != "CustomTool" {
		t.Errorf("expected 'CustomTool', got %q", entries[0].Summary)
	}
}

func TestFormatEntry_ToolUse(t *testing.T) {
	tests := []struct {
		tool    string
		summary string
		want    string
	}{
		{"Edit", "Edit: model.go", "Editing model.go"},
		{"Read", "Read: main.go", "Reading main.go"},
		{"Write", "Write: output.txt", "Writing output.txt"},
		{"Bash", "Bash: go test ./...", "Running go test ./..."},
		{"Grep", "Grep: func.*Test", "Searching func.*Test"},
		{"Glob", "Glob: **/*.go", "Finding **/*.go"},
		{"CustomTool", "CustomTool", "CustomTool"},
	}

	for _, tt := range tests {
		e := Entry{Type: "tool_use", Tool: tt.tool, Summary: tt.summary}
		got := FormatEntry(e)
		if got != tt.want {
			t.Errorf("FormatEntry(%s) = %q, want %q", tt.tool, got, tt.want)
		}
	}
}

func TestFormatEntry_Text(t *testing.T) {
	e := Entry{Type: "text", Summary: "I've updated the imports"}
	got := FormatEntry(e)
	if got != "I've updated the imports" {
		t.Errorf("FormatEntry(text) = %q, want %q", got, "I've updated the imports")
	}
}

func TestTailFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/a.go"}}]},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/tmp/b.go"}}]},"timestamp":"2025-01-15T10:01:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done!"}]},"timestamp":"2025-01-15T10:02:00Z"}`,
	}

	err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	entries := TailFile(path, 10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Newest first
	if entries[0].Summary != "Done!" {
		t.Errorf("first entry should be newest, got %q", entries[0].Summary)
	}
	if entries[1].Tool != "Edit" {
		t.Errorf("second entry should be Edit, got %q", entries[1].Tool)
	}
	if entries[2].Tool != "Read" {
		t.Errorf("third entry should be Read, got %q", entries[2].Tool)
	}
}

func TestTailFile_MaxEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/a.go"}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/tmp/b.go"}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"All done"}]}}`,
	}

	err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	entries := TailFile(path, 2)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Should be the newest 2
	if entries[0].Summary != "All done" {
		t.Errorf("first entry should be 'All done', got %q", entries[0].Summary)
	}
	if entries[1].Tool != "Edit" {
		t.Errorf("second entry should be Edit, got %q", entries[1].Tool)
	}
}

func TestTailFile_NonexistentFile(t *testing.T) {
	entries := TailFile("/nonexistent/file.jsonl", 10)
	if entries != nil {
		t.Errorf("expected nil for nonexistent file, got %v", entries)
	}
}

func TestTailFile_DiscardsPartialLine(t *testing.T) {
	// Create a file larger than 32KB to trigger the seek behavior.
	dir := t.TempDir()
	path := filepath.Join(dir, "big.jsonl")

	// Write padding lines to exceed 32KB, then a valid assistant line at the end.
	var buf strings.Builder
	// Each padding line is a valid user message, ~200 bytes. Need >32KB = 32768 bytes.
	for i := 0; i < 200; i++ {
		buf.WriteString(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"padding line number `)
		buf.WriteString(strings.Repeat("x", 100))
		buf.WriteString(`"}]}}`)
		buf.WriteByte('\n')
	}
	// Add a final assistant line.
	buf.WriteString(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"final entry"}]}}`)
	buf.WriteByte('\n')

	err := os.WriteFile(path, []byte(buf.String()), 0644)
	if err != nil {
		t.Fatal(err)
	}

	entries := TailFile(path, 10)
	// Should find at least the final entry and not crash on partial first line.
	found := false
	for _, e := range entries {
		if e.Summary == "final entry" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find 'final entry' in results")
	}
}
