package naming

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseResponse_SKIP(t *testing.T) {
	if got := parseResponse("SKIP"); got != "" {
		t.Errorf("SKIP → %q, want empty", got)
	}
	if got := parseResponse("skip"); got != "" {
		t.Errorf("skip → %q, want empty", got)
	}
	if got := parseResponse("  SKIP  "); got != "" {
		t.Errorf("padded SKIP → %q, want empty", got)
	}
}

func TestParseResponse_Empty(t *testing.T) {
	if got := parseResponse(""); got != "" {
		t.Errorf("empty → %q, want empty", got)
	}
	if got := parseResponse("   \n  "); got != "" {
		t.Errorf("whitespace → %q, want empty", got)
	}
}

func TestParseResponse_Normal(t *testing.T) {
	if got := parseResponse("config-sync autopull setup"); got != "config-sync autopull setup" {
		t.Errorf("got %q", got)
	}
}

func TestParseResponse_Trimmed(t *testing.T) {
	if got := parseResponse("  auth middleware migration  "); got != "auth middleware migration" {
		t.Errorf("got %q", got)
	}
}

func TestParseResponse_MultilineFirstLineOnly(t *testing.T) {
	input := "first line name\nsecond line explanation\nthird line"
	if got := parseResponse(input); got != "first line name" {
		t.Errorf("got %q, want %q", got, "first line name")
	}
}

func TestTailFileLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	var lines []string
	for i := range 100 {
		lines = append(lines, strings.Repeat("x", 10)+"_"+string(rune('0'+i%10)))
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)

	got := TailFileLines(path, 5)
	gotLines := strings.Split(got, "\n")
	if len(gotLines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(gotLines))
	}
}

func TestTailFileLines_MissingFile(t *testing.T) {
	got := TailFileLines("/nonexistent/file.txt", 10)
	if got != "" {
		t.Errorf("missing file should return empty, got %q", got)
	}
}

func TestTailFileLines_SmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3"), 0644)

	got := TailFileLines(path, 10)
	gotLines := strings.Split(got, "\n")
	if len(gotLines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(gotLines), gotLines)
	}
}
