package tui

import (
	"strings"
	"testing"
	"time"

	"ccs/internal/activity"
	"ccs/internal/capture"
	"ccs/internal/types"
)

// wrapText wraps text to fit within maxWidth, respecting existing newlines.
// Moved from views.go — only used in tests.
func wrapText(s string, maxWidth int) []string {
	if maxWidth < 10 {
		maxWidth = 10
	}
	var result []string
	for _, paragraph := range strings.Split(s, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len(line)+1+len(w) > maxWidth {
				result = append(result, line)
				line = w
			} else {
				line += " " + w
			}
		}
		result = append(result, line)
	}
	return result
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		wantLen  int
		wantLine string // first line
	}{
		{"short text", "hello world", 80, 1, "hello world"},
		{"wraps at width", "hello world foo bar", 11, 2, "hello world"},
		{"preserves newlines", "line1\nline2", 80, 2, "line1"},
		{"empty string", "", 80, 1, ""},
		{"minimum width enforced", "hello", 5, 1, "hello"},
		{"very narrow", "hello world", 1, 2, "hello"}, // min width is 10
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapText(tt.input, tt.width)
			if len(result) != tt.wantLen {
				t.Errorf("wrapText(%q, %d) returned %d lines, want %d: %v",
					tt.input, tt.width, len(result), tt.wantLen, result)
			}
			if len(result) > 0 && result[0] != tt.wantLine {
				t.Errorf("first line = %q, want %q", result[0], tt.wantLine)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"just now", now.Add(-30 * time.Second), "just now"},
		{"minutes", now.Add(-45 * time.Minute), "45m"},
		{"hours only", now.Add(-2 * time.Hour), "2h"},
		{"hours and minutes", now.Add(-2*time.Hour - 30*time.Minute), "2h 30m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.t)
			if got != tt.want {
				t.Errorf("formatDuration() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1024, "1 KB"},
		{2048, "2 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestGridPosition(t *testing.T) {
	grid := [][]int{
		{0, 1, 2},
		{3, 4},
		{5},
	}

	tests := []struct {
		idx  int
		wR   int
		wC   int
	}{
		{0, 0, 0},
		{2, 0, 2},
		{3, 1, 0},
		{4, 1, 1},
		{5, 2, 0},
	}

	for _, tt := range tests {
		r, c := gridPosition(grid, tt.idx)
		if r != tt.wR || c != tt.wC {
			t.Errorf("gridPosition(grid, %d) = (%d, %d), want (%d, %d)",
				tt.idx, r, c, tt.wR, tt.wC)
		}
	}
}

func TestSortFiltered(t *testing.T) {
	now := time.Now()
	m := &Model{
		filtered: []types.Session{
			{Title: "B session", LastActive: now.Add(-1 * time.Hour), ContextPct: 50, FileSize: 1000},
			{Title: "A session", LastActive: now.Add(-2 * time.Hour), ContextPct: 80, FileSize: 3000},
			{Title: "C session", LastActive: now, ContextPct: 20, FileSize: 2000},
		},
		sortField: types.SortByName,
		sortDir:   types.SortDesc,
	}

	m.sortFiltered()

	// SortByName + SortDesc: less = (a < b), no flip → alphabetical A, B, C
	if m.filtered[0].Title != "A session" {
		t.Errorf("first = %q, want A session", m.filtered[0].Title)
	}
	if m.filtered[2].Title != "C session" {
		t.Errorf("last = %q, want C session", m.filtered[2].Title)
	}

	// Switch to ascending (flips: !less → reverse alphabetical)
	m.sortDir = types.SortAsc
	m.sortFiltered()

	if m.filtered[0].Title != "C session" {
		t.Errorf("first = %q, want C session", m.filtered[0].Title)
	}
}

func TestRenderDetailHeight(t *testing.T) {
	// Verify that renderDetail always produces exactly detailPaneLines() lines,
	// regardless of content. Long pane capture lines must not wrap.
	cfg := &types.Config{}
	m := Model{
		config:      cfg,
		focus:       FocusSessions,
		width:       100,
		height:      40,
		paneContent: make(map[string]capture.PaneSnapshot),
		activities:  make(map[string][]activity.Entry),
	}

	longLine := strings.Repeat("x", 200) // much wider than any detail pane
	tests := []struct {
		name    string
		session types.Session
		pane    string // pane capture content (empty = no capture)
	}{
		{
			name:    "no content",
			session: types.Session{ID: "s1", ProjectName: "test", Title: "title"},
		},
		{
			name:    "short pane capture",
			session: types.Session{ID: "s2", ProjectName: "test", Title: "title", ActiveSource: types.SourceTmux},
			pane:    "line1\nline2\nline3",
		},
		{
			name:    "long pane lines that should be truncated",
			session: types.Session{ID: "s3", ProjectName: "test", Title: "title", ActiveSource: types.SourceTmux},
			pane:    longLine + "\n" + longLine + "\n" + longLine + "\n" + longLine + "\n" + longLine,
		},
		{
			name:    "inactive with long pane capture",
			session: types.Session{ID: "s4", ProjectName: "test", Title: "title"},
			pane:    longLine + "\n" + longLine + "\n" + longLine,
		},
		{
			name:    "long project dir",
			session: types.Session{ID: "s5", ProjectName: "very-long-project-name", ProjectDir: "/home/mse/Projects/very-long-project-name", Title: strings.Repeat("w", 100)},
		},
		{
			name:    "long session name + title",
			session: types.Session{ID: "s6", ProjectName: "proj", SessionName: "my-long-session-name", Title: strings.Repeat("t", 100)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.filtered = []types.Session{tt.session}
			m.sessionIdx = 0
			if tt.pane != "" {
				m.paneContent[tt.session.ID] = capture.PaneSnapshot{Content: tt.pane}
			} else {
				delete(m.paneContent, tt.session.ID)
			}

			rendered := m.renderDetail(tt.session)
			actualLines := strings.Count(rendered, "\n") + 1
			expected := m.detailPaneLines()

			if actualLines != expected {
				t.Errorf("renderDetail produced %d lines, want %d (detailPaneLines)\nrendered:\n%s",
					actualLines, expected, rendered)
			}
		})
	}
}

func TestContextStyle(t *testing.T) {
	// Just verify it doesn't panic for edge values
	_ = contextStyle(0)
	_ = contextStyle(59)
	_ = contextStyle(60)
	_ = contextStyle(79)
	_ = contextStyle(80)
	_ = contextStyle(100)
}
