package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"

	"ccs/internal/activity"
	"ccs/internal/capture"
	"ccs/internal/state"
	"ccs/internal/types"
)

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		wantLen  int
		wantLine string
	}{
		{"short text", "hello world", 80, 1, "hello world"},
		{"wraps at width", "hello world foo bar", 11, 2, "hello world"},
		{"empty string", "", 80, 1, ""},
		{"single word narrow", "hello", 3, 1, "hello"},
		{"two words narrow", "hello world", 1, 2, "hello"},
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
		{"1 day", now.Add(-25 * time.Hour), "1d"},
		{"2 days", now.Add(-49 * time.Hour), "2d"},
		{"7 days", now.Add(-168 * time.Hour), "7d"},
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

func TestSortSlice(t *testing.T) {
	now := time.Now()
	m := &Model{
		sortField: types.SortByName,
		sortDir:   types.SortDesc,
	}

	sessions := []types.Session{
		{Title: "B session", LastActive: now.Add(-1 * time.Hour), ContextPct: 50, FileSize: 1000},
		{Title: "A session", LastActive: now.Add(-2 * time.Hour), ContextPct: 80, FileSize: 3000},
		{Title: "C session", LastActive: now, ContextPct: 20, FileSize: 2000},
	}

	m.sortSlice(sessions)

	if sessions[0].Title != "A session" {
		t.Errorf("first = %q, want A session", sessions[0].Title)
	}
	if sessions[2].Title != "C session" {
		t.Errorf("last = %q, want C session", sessions[2].Title)
	}

	m.sortDir = types.SortAsc
	m.sortSlice(sessions)
	if sessions[0].Title != "C session" {
		t.Errorf("first = %q, want C session", sessions[0].Title)
	}
}

func tempState(t *testing.T) *state.Store {
	t.Helper()
	return state.LoadFromDir(t.TempDir())
}

func TestDisplayName(t *testing.T) {
	st := tempState(t)
	m := &Model{state: st}

	// Fallback to Title
	s := types.Session{ID: "test1", Title: "fix auth bug", SessionName: ""}
	if got := m.displayName(s); got != "fix auth bug" {
		t.Errorf("displayName with title only: got %q, want %q", got, "fix auth bug")
	}

	// SessionName overrides Title
	s.SessionName = "session-rename"
	if got := m.displayName(s); got != "session-rename" {
		t.Errorf("displayName with session name: got %q, want %q", got, "session-rename")
	}

	// State store name overrides SessionName
	st.MarkOpen("test1")
	st.SetName("test1", "auto named", "auto")
	if got := m.displayName(s); got != "auto named" {
		t.Errorf("displayName with auto name: got %q, want %q", got, "auto named")
	}

	// Manual name overrides auto
	st.SetName("test1", "manual name", "manual")
	if got := m.displayName(s); got != "manual name" {
		t.Errorf("displayName with manual name: got %q, want %q", got, "manual name")
	}
}

func TestRenderDetailHeight(t *testing.T) {
	cfg := &types.Config{}
	st := tempState(t)
	m := Model{
		config:      cfg,
		width:       100,
		height:      40,
		paneContent: make(map[string]capture.PaneSnapshot),
		activities:  make(map[string][]activity.Entry),
		state:       st,
	}

	longLine := strings.Repeat("x", 200)
	tests := []struct {
		name    string
		session types.Session
		pane    string
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
			name:    "long pane lines",
			session: types.Session{ID: "s3", ProjectName: "test", Title: "title", ActiveSource: types.SourceTmux},
			pane:    longLine + "\n" + longLine + "\n" + longLine + "\n" + longLine + "\n" + longLine,
		},
		{
			name:    "inactive with pane capture",
			session: types.Session{ID: "s4", ProjectName: "test", Title: "title"},
			pane:    longLine + "\n" + longLine + "\n" + longLine,
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
	_ = contextStyle(0)
	_ = contextStyle(59)
	_ = contextStyle(60)
	_ = contextStyle(79)
	_ = contextStyle(80)
	_ = contextStyle(100)
}

func TestMarkDoneImmediatelyUpdatesFiltered(t *testing.T) {
	st := tempState(t)
	st.MarkOpen("o1")
	st.MarkOpen("o2")

	sessions := []types.Session{
		{ID: "o1", LastActive: time.Now().Add(-time.Hour)},
		{ID: "o2", LastActive: time.Now().Add(-2 * time.Hour)},
	}
	// Set initial StateStatus via computeStateStatuses (no tracker = no active sessions)
	computeStateStatuses(sessions, nil, st)

	m := &Model{
		sessions:  sessions,
		config:    &types.Config{},
		sortField: types.SortByTime,
		sortDir:   types.SortDesc,
		state:     st,
		filter:    textinput.New(),
	}
	m.applyFilter()

	if len(m.filtered) != 2 {
		t.Fatalf("expected 2 open sessions, got %d", len(m.filtered))
	}

	// Mark first session done — should immediately disappear from filtered
	m.sessionIdx = 0
	m.state.MarkDone(m.filtered[0].ID)
	computeStateStatuses(m.sessions, m.tracker, m.state)
	m.applyFilter()

	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 open session after marking done, got %d", len(m.filtered))
	}
	if m.filtered[0].ID != "o2" {
		t.Errorf("remaining session should be o2, got %s", m.filtered[0].ID)
	}

	// The done session should have StatusDone in m.sessions
	for _, s := range m.sessions {
		if s.ID == "o1" && s.StateStatus != types.StatusDone {
			t.Errorf("o1 StateStatus = %v, want StatusDone", s.StateStatus)
		}
	}
}

func TestReopenImmediatelyUpdatesFiltered(t *testing.T) {
	st := tempState(t)
	st.MarkOpen("o1")
	st.MarkDone("d1")

	sessions := []types.Session{
		{ID: "o1", LastActive: time.Now().Add(-time.Hour)},
		{ID: "d1", LastActive: time.Now().Add(-2 * time.Hour)},
	}
	computeStateStatuses(sessions, nil, st)

	m := &Model{
		sessions:          sessions,
		config:            &types.Config{},
		sortField:         types.SortByTime,
		sortDir:           types.SortDesc,
		state:             st,
		filter:            textinput.New(),
		showDoneUntracked: true,
	}
	m.applyFilter()

	if len(m.filtered) != 2 {
		t.Fatalf("expected 2 sessions (1 open + 1 done), got %d", len(m.filtered))
	}

	// Reopen the done session
	m.state.Reopen("d1")
	computeStateStatuses(m.sessions, m.tracker, m.state)
	m.applyFilter()

	open := m.openSessions()
	if len(open) != 2 {
		t.Errorf("expected 2 open sessions after reopen, got %d", len(open))
	}
}

func TestActiveOpenPartitioning(t *testing.T) {
	st := tempState(t)
	m := &Model{
		sessions: []types.Session{
			{ID: "a1", StateStatus: types.StatusActive, LastActive: time.Now()},
			{ID: "o1", StateStatus: types.StatusOpen, LastActive: time.Now().Add(-time.Hour)},
			{ID: "o2", StateStatus: types.StatusOpen, LastActive: time.Now().Add(-2 * time.Hour)},
			{ID: "d1", StateStatus: types.StatusDone, LastActive: time.Now().Add(-3 * time.Hour)},
			{ID: "u1", StateStatus: types.StatusUntracked, LastActive: time.Now().Add(-4 * time.Hour)},
		},
		config:    &types.Config{},
		sortField: types.SortByTime,
		sortDir:   types.SortDesc,
		state:     st,
		filter:    textinput.New(),
	}

	m.applyFilter()

	// Without showDoneUntracked, should have active + open only
	if len(m.filtered) != 3 {
		t.Fatalf("expected 3 filtered (1 active + 2 open), got %d", len(m.filtered))
	}
	if m.filtered[0].ID != "a1" {
		t.Errorf("first should be active, got %s", m.filtered[0].ID)
	}

	active := m.activeSessions()
	if len(active) != 1 {
		t.Errorf("expected 1 active, got %d", len(active))
	}

	open := m.openSessions()
	if len(open) != 2 {
		t.Errorf("expected 2 open, got %d", len(open))
	}

	// With showDoneUntracked
	m.showDoneUntracked = true
	m.applyFilter()
	if len(m.filtered) != 5 {
		t.Fatalf("expected 5 filtered with done/untracked, got %d", len(m.filtered))
	}
}
