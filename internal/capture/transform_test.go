package capture

import (
	"strings"
	"testing"
)

func TestStripStatusBar(t *testing.T) {
	// Create a box-drawing separator line
	separator := strings.Repeat("─", 40)
	content := "line1\nline2\nline3\n" + separator + "\nHUD line 1\nHUD line 2"

	result := stripStatusBar(content)
	if strings.Contains(result, "HUD") {
		t.Errorf("expected HUD lines stripped, got %q", result)
	}
	if !strings.Contains(result, "line3") {
		t.Errorf("expected content lines preserved, got %q", result)
	}
}

func TestStripStatusBar_NoSeparator(t *testing.T) {
	content := "line1\nline2\nline3"
	result := stripStatusBar(content)
	if result != content {
		t.Errorf("expected unchanged content, got %q", result)
	}
}

func TestIsSpinnerLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"  ✻ Thinking...", true},
		{"  * Contemplating...", true},
		{"  · Working...", true},
		{"regular text", false},
		{"", false},
		{"   ", false},
	}
	for _, tt := range tests {
		if got := IsSpinnerLine(tt.line); got != tt.want {
			t.Errorf("IsSpinnerLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestIsSpinnerLine_Braille(t *testing.T) {
	// Braille characters U+2800-U+28FF
	if !IsSpinnerLine("⠋ Loading") {
		t.Error("expected braille spinner to be detected")
	}
}

func TestCollapseTaskList(t *testing.T) {
	content := "header\n✓ task 1\n✓ task 2\n■ active task\n□ pending task\nfooter"
	result := collapseTaskList(content)

	if !strings.Contains(result, "header") {
		t.Error("expected header preserved")
	}
	if !strings.Contains(result, "footer") {
		t.Error("expected footer preserved")
	}
	if !strings.Contains(result, "active task") {
		t.Error("expected active task preserved")
	}
	if strings.Contains(result, "pending task") {
		t.Error("expected pending task removed")
	}
	if strings.Contains(result, "2/4 done") || strings.Contains(result, "  2/4") {
		// Good — summary line present
	}
}

func TestTodoLineMarker(t *testing.T) {
	tests := []struct {
		line   string
		marker string
		isTodo bool
	}{
		{"✓ done task", "done", true},
		{"✔ also done", "done", true},
		{"■ active task", "active", true},
		{"▪ also active", "active", true},
		{"□ pending task", "pending", true},
		{"regular text", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		marker, isTodo := TodoLineMarker(tt.line)
		if marker != tt.marker || isTodo != tt.isTodo {
			t.Errorf("TodoLineMarker(%q) = (%q, %v), want (%q, %v)",
				tt.line, marker, isTodo, tt.marker, tt.isTodo)
		}
	}
}

func TestStripTrailingNoise(t *testing.T) {
	content := "real content\n\n\n  ✻ Thinking...\n\n"
	result := stripTrailingNoise(content)
	if result != "real content" {
		t.Errorf("expected 'real content', got %q", result)
	}
}

func TestTransformPaneContent(t *testing.T) {
	// Integration test: all transforms applied in order
	separator := strings.Repeat("─", 40)
	content := "line1\n✓ done\n■ active\n□ pending\n" + separator + "\nHUD\n\n✻ Thinking...\n"

	result := TransformPaneContent(content)
	if strings.Contains(result, "HUD") {
		t.Error("expected HUD stripped")
	}
	if strings.Contains(result, "pending") {
		t.Error("expected pending task removed")
	}
	if !strings.Contains(result, "active") {
		t.Error("expected active task kept")
	}
}
