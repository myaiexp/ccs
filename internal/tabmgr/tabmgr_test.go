package tabmgr

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestFormatTabBar(t *testing.T) {
	tabs := []Tab{
		{WindowID: "@1", ProjectName: "proj-a", DisplayName: "fix auth"},
		{WindowID: "@2", ProjectName: "proj-b", DisplayName: "add tests"},
		{WindowID: "@3", ProjectName: "proj-c", DisplayName: "refactor"},
	}
	currentWindowID := "@2"

	result := FormatTabBar(tabs, currentWindowID, 200)

	// Dashboard should be first
	if !strings.Contains(result, "⌂") {
		t.Error("expected dashboard icon ⌂ in output")
	}

	// Current tab should have ▸ marker
	if !strings.Contains(result, "▸") {
		t.Error("expected ▸ marker for current tab")
	}

	// All project names should appear
	for _, tab := range tabs {
		if !strings.Contains(result, tab.ProjectName) {
			t.Errorf("expected %q in output, got: %s", tab.ProjectName, result)
		}
	}

	// The current tab (proj-b) should be bold with ▸
	if !strings.Contains(result, "▸ proj-b: add tests") {
		t.Errorf("expected current tab marker on proj-b, got: %s", result)
	}
}

func TestFormatTabBarOverflow(t *testing.T) {
	// Create many tabs to exceed maxWidth
	var tabs []Tab
	for i := 0; i < 20; i++ {
		tabs = append(tabs, Tab{
			WindowID:    "@" + string(rune('1'+i)),
			ProjectName: "long-project-name",
			DisplayName: "some-display-name",
		})
	}

	result := FormatTabBar(tabs, "@999", 80)

	if !strings.Contains(result, "+") && !strings.Contains(result, "more") {
		t.Errorf("expected overflow indicator '+N more' in output, got: %s", result)
	}
}

func TestFormatTabBarAttention(t *testing.T) {
	tabs := []Tab{
		{WindowID: "@1", ProjectName: "proj-a"},
		{WindowID: "@2", ProjectName: "proj-b", Attention: "waiting"},
		{WindowID: "@3", ProjectName: "proj-c", Attention: "error"},
	}

	result := FormatTabBar(tabs, "@1", 200)

	// Waiting tab should have yellow ●
	if !strings.Contains(result, "#[fg=yellow]●") {
		t.Errorf("expected yellow attention dot for waiting tab, got: %s", result)
	}

	// Error tab should have red ●
	if !strings.Contains(result, "#[fg=red]●") {
		t.Errorf("expected red attention dot for error tab, got: %s", result)
	}

	// Non-attention tab should NOT have ●
	// proj-a has no attention, verify ● only appears after proj-b and proj-c
	parts := strings.Split(result, "|")
	for _, part := range parts {
		if strings.Contains(part, "proj-a") && strings.Contains(part, "●") {
			t.Errorf("proj-a should not have attention indicator, got: %s", part)
		}
	}
}

func TestFormatAttentionSummaryEmpty(t *testing.T) {
	tabs := []Tab{
		{WindowID: "@1", ProjectName: "proj-a"},
		{WindowID: "@2", ProjectName: "proj-b"},
	}

	result := FormatAttentionSummary(tabs)
	if result != "" {
		t.Errorf("expected empty string when no tabs need attention, got: %q", result)
	}
}

func TestFormatAttentionSummary(t *testing.T) {
	tabs := []Tab{
		{WindowID: "@1", ProjectName: "proj-a", Attention: "waiting"},
		{WindowID: "@2", ProjectName: "proj-b"},
		{WindowID: "@3", ProjectName: "proj-c", Attention: "error"},
	}

	result := FormatAttentionSummary(tabs)

	if result == "" {
		t.Fatal("expected non-empty attention summary")
	}

	// Should mention both attention sessions
	if !strings.Contains(result, "proj-a") {
		t.Errorf("expected proj-a in attention summary, got: %s", result)
	}
	if !strings.Contains(result, "proj-c") {
		t.Errorf("expected proj-c in attention summary, got: %s", result)
	}

	// Should contain attention descriptions
	if !strings.Contains(result, "waiting for input") {
		t.Errorf("expected 'waiting for input' in summary, got: %s", result)
	}
	if !strings.Contains(result, "has error") {
		t.Errorf("expected 'has error' in summary, got: %s", result)
	}

	// proj-b should NOT be in the summary
	if strings.Contains(result, "proj-b") {
		t.Errorf("proj-b has no attention, should not be in summary, got: %s", result)
	}
}

func TestFormatAttentionSummaryPermission(t *testing.T) {
	tabs := []Tab{
		{WindowID: "@1", ProjectName: "proj-a", Attention: "permission"},
	}

	result := FormatAttentionSummary(tabs)

	if !strings.Contains(result, "needs permission") {
		t.Errorf("expected 'needs permission' in summary, got: %s", result)
	}
	if !strings.Contains(result, "#[fg=colour208]") {
		t.Errorf("expected orange color for permission attention, got: %s", result)
	}
}

func TestFormatAttentionSummaryDisplayName(t *testing.T) {
	tabs := []Tab{
		{WindowID: "@1", ProjectName: "proj-a", DisplayName: "my-task", Attention: "waiting"},
	}

	result := FormatAttentionSummary(tabs)

	// Should use DisplayName over ProjectName when available
	if !strings.Contains(result, "my-task") {
		t.Errorf("expected display name 'my-task' in summary, got: %s", result)
	}
}

func TestTabsReturnsCopy(t *testing.T) {
	m := &Manager{
		pending: make(map[string]pendingCallback),
		tabs: []Tab{
			{WindowID: "@1", ProjectName: "proj-a"},
		},
	}

	tabs := m.Tabs()
	tabs[0].ProjectName = "mutated"

	// Original should be unchanged
	origTabs := m.Tabs()
	if origTabs[0].ProjectName != "proj-a" {
		t.Error("Tabs() should return a copy, but original was mutated")
	}
}

func TestHandleExitRemovesTab(t *testing.T) {
	m := &Manager{
		pending: make(map[string]pendingCallback),
		tabs: []Tab{
			{WindowID: "@1", ProjectName: "proj-a", SessionID: "sess-1"},
			{WindowID: "@2", ProjectName: "proj-b", SessionID: "sess-2"},
			{WindowID: "@3", ProjectName: "proj-c", SessionID: "sess-3"},
		},
	}

	m.HandleExit("@2")

	tabs := m.Tabs()
	if len(tabs) != 2 {
		t.Fatalf("expected 2 tabs after exit, got %d", len(tabs))
	}

	for _, tab := range tabs {
		if tab.WindowID == "@2" {
			t.Error("expected @2 to be removed")
		}
	}
}

func TestHandleExitNonexistentWindow(t *testing.T) {
	m := &Manager{
		pending: make(map[string]pendingCallback),
		tabs: []Tab{
			{WindowID: "@1", ProjectName: "proj-a"},
		},
	}

	// Should not panic
	m.HandleExit("@999")

	tabs := m.Tabs()
	if len(tabs) != 1 {
		t.Errorf("expected 1 tab unchanged, got %d", len(tabs))
	}
}

func TestHandleExitStoresPendingCallback(t *testing.T) {
	m := &Manager{
		pending: make(map[string]pendingCallback),
		tabs: []Tab{
			{WindowID: "@1", ProjectName: "proj-a", SessionID: "sess-1", OnDone: "echo done", ProjectDir: "/proj/a"},
		},
	}

	m.HandleExit("@1")

	// Tab should be removed
	tabs := m.Tabs()
	if len(tabs) != 0 {
		t.Fatalf("expected 0 tabs after exit, got %d", len(tabs))
	}

	// Pending callback should be stored
	m.mu.Lock()
	pc, ok := m.pending["sess-1"]
	m.mu.Unlock()

	if !ok {
		t.Fatal("expected pending callback for sess-1")
	}
	if pc.OnDone != "echo done" {
		t.Errorf("expected OnDone 'echo done', got %q", pc.OnDone)
	}
	if pc.ProjectDir != "/proj/a" {
		t.Errorf("expected ProjectDir '/proj/a', got %q", pc.ProjectDir)
	}
}

func TestHandleExitNoCallbackIfNoOnDone(t *testing.T) {
	m := &Manager{
		pending: make(map[string]pendingCallback),
		tabs: []Tab{
			{WindowID: "@1", ProjectName: "proj-a", SessionID: "sess-1"},
		},
	}

	m.HandleExit("@1")

	m.mu.Lock()
	_, ok := m.pending["sess-1"]
	m.mu.Unlock()

	if ok {
		t.Error("should not store pending callback when OnDone is empty")
	}
}

func TestFirePendingCallback(t *testing.T) {
	m := &Manager{
		pending: map[string]pendingCallback{
			"sess-1": {
				SessionID:  "sess-1",
				ProjectDir: "/proj/a",
				OnDone:     "true", // no-op command that succeeds
			},
		},
	}

	fired := m.FirePendingCallback("sess-1", "completed task")
	if !fired {
		t.Error("expected callback to fire")
	}

	// Should be cleared from pending
	m.mu.Lock()
	_, ok := m.pending["sess-1"]
	m.mu.Unlock()

	if ok {
		t.Error("expected pending callback to be cleared after firing")
	}
}

func TestFirePendingCallbackNonexistent(t *testing.T) {
	m := &Manager{
		pending: make(map[string]pendingCallback),
	}

	fired := m.FirePendingCallback("nonexistent", "summary")
	if fired {
		t.Error("should not fire callback for nonexistent session")
	}
}

func TestPendingCallbackSessionIDs(t *testing.T) {
	m := &Manager{
		pending: map[string]pendingCallback{
			"sess-1": {SessionID: "sess-1", OnDone: "cmd1"},
			"sess-2": {SessionID: "sess-2", OnDone: "cmd2"},
		},
	}

	ids := m.PendingCallbackSessionIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 pending IDs, got %d", len(ids))
	}

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	if !idSet["sess-1"] || !idSet["sess-2"] {
		t.Errorf("expected sess-1 and sess-2, got %v", ids)
	}
}

func TestSyncFromTrackerPopulatesSessionID(t *testing.T) {
	// We can't call SyncFromTracker without a real tracker + tmux,
	// so we test the underlying sync logic directly:
	// Given a windowToSession map, tabs with empty SessionID get populated.
	m := &Manager{
		pending: make(map[string]pendingCallback),
		tabs: []Tab{
			{WindowID: "@1", ProjectName: "proj-a"},
			{WindowID: "@2", ProjectName: "proj-b", SessionID: "already-set"},
			{WindowID: "@3", ProjectName: "proj-c"},
		},
	}

	// Simulate what SyncFromTracker does internally
	m.mu.Lock()
	windowToSession := map[string]string{
		"@1": "sess-from-tracker",
		"@3": "sess-3-from-tracker",
	}
	for i := range m.tabs {
		if m.tabs[i].SessionID == "" {
			if sessID, ok := windowToSession[m.tabs[i].WindowID]; ok {
				m.tabs[i].SessionID = sessID
			}
		}
	}
	m.mu.Unlock()

	tabs := m.Tabs()

	if tabs[0].SessionID != "sess-from-tracker" {
		t.Errorf("expected @1 to get sess-from-tracker, got %q", tabs[0].SessionID)
	}
	if tabs[1].SessionID != "already-set" {
		t.Errorf("expected @2 to keep already-set, got %q", tabs[1].SessionID)
	}
	if tabs[2].SessionID != "sess-3-from-tracker" {
		t.Errorf("expected @3 to get sess-3-from-tracker, got %q", tabs[2].SessionID)
	}
}

func TestUpdateAttention(t *testing.T) {
	m := &Manager{
		pending: make(map[string]pendingCallback),
		tabs: []Tab{
			{WindowID: "@1", ProjectName: "proj-a"},
			{WindowID: "@2", ProjectName: "proj-b"},
		},
	}

	m.UpdateAttention("@1", "waiting")
	m.UpdateAttention("@2", "error")

	tabs := m.Tabs()
	if tabs[0].Attention != "waiting" {
		t.Errorf("expected @1 attention 'waiting', got %q", tabs[0].Attention)
	}
	if tabs[1].Attention != "error" {
		t.Errorf("expected @2 attention 'error', got %q", tabs[1].Attention)
	}

	// Clear attention
	m.UpdateAttention("@1", "")
	tabs = m.Tabs()
	if tabs[0].Attention != "" {
		t.Errorf("expected @1 attention cleared, got %q", tabs[0].Attention)
	}
}

func TestUpdateAttentionNonexistent(t *testing.T) {
	m := &Manager{
		pending: make(map[string]pendingCallback),
		tabs: []Tab{
			{WindowID: "@1", ProjectName: "proj-a"},
		},
	}

	// Should not panic
	m.UpdateAttention("@999", "waiting")
}

func TestPlainWidth(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"hello", 5},
		{"#[fg=red]hello#[default]", 5},
		{"#[bold]▸ proj#[nobold]", 6}, // "▸ proj" = 6 runes
		{"", 0},
		{"no tags", 7},
		{"#[fg=yellow]●#[default]", 1}, // "●" = 1 rune
	}

	for _, tt := range tests {
		got := plainWidth(tt.input)
		if got != tt.expected {
			t.Errorf("plainWidth(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestIsClaudeProcess(t *testing.T) {
	// Test with current process (not claude)
	if isClaudeProcess(os.Getpid()) {
		// This is OK if the test runner happens to be called "claude"
		// but typically it won't be
		t.Log("current process detected as claude (unexpected but possible)")
	}

	// Test with invalid PID
	if isClaudeProcess(999999999) {
		t.Error("invalid PID should not be detected as claude process")
	}
}

func TestHandleExitTimeoutCallback(t *testing.T) {
	// This test verifies the timeout goroutine behavior.
	// We create a manager with a tab that has OnDone set to "true" (no-op),
	// call HandleExit, and verify the pending callback exists initially
	// then gets cleared by the timeout.
	m := &Manager{
		pending: make(map[string]pendingCallback),
		tabs: []Tab{
			{WindowID: "@1", ProjectName: "proj-a", SessionID: "sess-1", OnDone: "true", ProjectDir: "/proj/a"},
		},
	}

	m.HandleExit("@1")

	// Immediately after exit, pending should exist
	m.mu.Lock()
	_, ok := m.pending["sess-1"]
	m.mu.Unlock()
	if !ok {
		t.Fatal("expected pending callback immediately after exit")
	}

	// Wait for timeout (10s + buffer) — this is a long test
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	time.Sleep(11 * time.Second)

	m.mu.Lock()
	_, ok = m.pending["sess-1"]
	m.mu.Unlock()
	if ok {
		t.Error("expected pending callback to be cleared after 10s timeout")
	}
}

func TestNew(t *testing.T) {
	m := New("ccs", nil, nil, []string{"--flag"})

	if m.sessionName != "ccs" {
		t.Errorf("expected sessionName 'ccs', got %q", m.sessionName)
	}
	if len(m.claudeFlags) != 1 || m.claudeFlags[0] != "--flag" {
		t.Errorf("expected claudeFlags [--flag], got %v", m.claudeFlags)
	}
	if m.pending == nil {
		t.Error("pending map should be initialized")
	}
	if len(m.Tabs()) != 0 {
		t.Error("new manager should have no tabs")
	}
}

