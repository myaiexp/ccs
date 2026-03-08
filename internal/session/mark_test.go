package session

import (
	"testing"
	"time"

	"ccs/internal/types"
)

func mkActive(dir string, starts ...time.Time) types.ActiveInfo {
	return types.ActiveInfo{
		ProjectDirs: map[string]types.ProjectActiveInfo{
			dir: {ProcessStarts: starts},
		},
	}
}

func TestMarkActive_SingleProcess(t *testing.T) {
	now := time.Now()
	pStart := now.Add(-1 * time.Hour)
	sessions := []types.Session{
		{ID: "match", ProjectDir: "/proj", CreatedAt: pStart.Add(3 * time.Second), LastActive: now},
		{ID: "old1", ProjectDir: "/proj", CreatedAt: pStart.Add(-2 * time.Hour), LastActive: now.Add(-1 * time.Hour)},
		{ID: "old2", ProjectDir: "/proj", CreatedAt: pStart.Add(-3 * time.Hour), LastActive: now.Add(-2 * time.Hour)},
	}

	MarkActiveSessions(sessions, mkActive("/proj", pStart))

	if !sessions[0].IsActive {
		t.Error("session created 3s after process start should be active")
	}
	if sessions[1].IsActive || sessions[2].IsActive {
		t.Error("old sessions should not be active")
	}
}

func TestMarkActive_MultipleProcesses(t *testing.T) {
	now := time.Now()
	p1 := now.Add(-60 * time.Minute)
	p2 := now.Add(-40 * time.Minute)
	p3 := now.Add(-20 * time.Minute)
	sessions := []types.Session{
		{ID: "s1", ProjectDir: "/proj", CreatedAt: p1.Add(2 * time.Second), LastActive: now},
		{ID: "s2", ProjectDir: "/proj", CreatedAt: p2.Add(3 * time.Second), LastActive: now.Add(-10 * time.Minute)},
		{ID: "s3", ProjectDir: "/proj", CreatedAt: p3.Add(1 * time.Second), LastActive: now.Add(-20 * time.Minute)},
		{ID: "s4", ProjectDir: "/proj", CreatedAt: p1.Add(-2 * time.Hour), LastActive: now.Add(-30 * time.Minute)},
		{ID: "s5", ProjectDir: "/proj", CreatedAt: p1.Add(-3 * time.Hour), LastActive: now.Add(-40 * time.Minute)},
	}

	MarkActiveSessions(sessions, mkActive("/proj", p1, p2, p3))

	for i, s := range sessions {
		if i < 3 && !s.IsActive {
			t.Errorf("session %d (%s) should be active", i, s.ID)
		}
		if i >= 3 && s.IsActive {
			t.Errorf("session %d (%s) should not be active", i, s.ID)
		}
	}
}

func TestMarkActive_FiltersOldSessions(t *testing.T) {
	now := time.Now()
	pStart := now.Add(-1 * time.Hour)
	sessions := []types.Session{
		{ID: "new", ProjectDir: "/proj", CreatedAt: pStart.Add(2 * time.Second), LastActive: now},
		{ID: "old", ProjectDir: "/proj", CreatedAt: pStart.Add(-24 * time.Hour), LastActive: pStart.Add(10 * time.Minute)},
	}

	MarkActiveSessions(sessions, mkActive("/proj", pStart, pStart.Add(5*time.Minute)))

	if !sessions[0].IsActive {
		t.Error("new session should be active")
	}
	if sessions[1].IsActive {
		t.Error("old session (created before process start) should not be active")
	}
}

func TestMarkActive_MultipleProjects(t *testing.T) {
	now := time.Now()
	pStart := now.Add(-1 * time.Hour)
	sessions := []types.Session{
		{ID: "a1", ProjectDir: "/projA", CreatedAt: pStart.Add(2 * time.Second), LastActive: now},
		{ID: "a2", ProjectDir: "/projA", CreatedAt: pStart.Add(-2 * time.Hour), LastActive: now.Add(-10 * time.Minute)},
		{ID: "b1", ProjectDir: "/projB", CreatedAt: pStart.Add(3 * time.Second), LastActive: now},
		{ID: "b2", ProjectDir: "/projB", CreatedAt: pStart.Add(20 * time.Minute), LastActive: now.Add(-10 * time.Minute)},
	}
	active := types.ActiveInfo{
		ProjectDirs: map[string]types.ProjectActiveInfo{
			"/projA": {ProcessStarts: []time.Time{pStart}},
			"/projB": {ProcessStarts: []time.Time{pStart, pStart.Add(20 * time.Minute)}},
		},
	}

	MarkActiveSessions(sessions, active)

	if !sessions[0].IsActive {
		t.Error("a1 should be active")
	}
	if sessions[1].IsActive {
		t.Error("a2 should not be active")
	}
	if !sessions[2].IsActive || !sessions[3].IsActive {
		t.Error("both projB sessions should be active")
	}
}

func TestMarkActive_NoProcesses(t *testing.T) {
	sessions := []types.Session{
		{ID: "s1", ProjectDir: "/proj", CreatedAt: time.Now(), LastActive: time.Now()},
	}
	active := types.ActiveInfo{
		ProjectDirs: make(map[string]types.ProjectActiveInfo),
	}

	MarkActiveSessions(sessions, active)

	if sessions[0].IsActive {
		t.Error("no processes — session should not be active")
	}
}

func TestMarkActive_MoreProcessesThanSessions(t *testing.T) {
	now := time.Now()
	p1 := now.Add(-1 * time.Hour)
	sessions := []types.Session{
		{ID: "s1", ProjectDir: "/proj", CreatedAt: p1.Add(2 * time.Second), LastActive: now},
	}

	MarkActiveSessions(sessions, mkActive("/proj", p1, p1.Add(10*time.Minute), p1.Add(20*time.Minute)))

	if !sessions[0].IsActive {
		t.Error("qualifying session should be active")
	}
}

func TestMarkActive_DeadProcessSessionNotMarked(t *testing.T) {
	// The Bitwarden scenario:
	// Process A started at 09:23, still running, never created a session
	// Process B started at ~09:34, created Bitwarden session at 09:34, exited
	// Now: 1 process running (A at 09:23), 1 session (created 09:34)
	// Session created 11 min after process A start → outside 2-min window → no match!
	now := time.Now()
	processA := now.Add(-3 * time.Hour)
	sessions := []types.Session{
		{
			ID:         "bitwarden",
			ProjectDir: "/home/mse",
			CreatedAt:  processA.Add(11 * time.Minute), // created by dead process B
			LastActive: processA.Add(18 * time.Minute), // finished long ago
		},
	}

	MarkActiveSessions(sessions, mkActive("/home/mse", processA))

	if sessions[0].IsActive {
		t.Error("session created 11min after process start should NOT match (outside 2-min window)")
	}
}

func TestMarkActive_RecentActivityPhase2(t *testing.T) {
	// Compacted session: creation time is old but mtime is very recent
	now := time.Now()
	pStart := now.Add(-2 * time.Hour)
	sessions := []types.Session{
		{
			ID:         "compacted",
			ProjectDir: "/proj",
			CreatedAt:  pStart.Add(-24 * time.Hour), // old creation
			LastActive: now.Add(-2 * time.Minute),   // modified 2 min ago
		},
	}

	MarkActiveSessions(sessions, mkActive("/proj", pStart))

	if !sessions[0].IsActive {
		t.Error("recently modified session should match via phase 2")
	}
}

func TestMarkActive_Phase1ThenPhase2(t *testing.T) {
	// 2 processes: one matches via creation time, one via recent activity
	now := time.Now()
	p1 := now.Add(-2 * time.Hour)
	p2 := now.Add(-1 * time.Hour)
	sessions := []types.Session{
		{ID: "created-match", ProjectDir: "/proj", CreatedAt: p1.Add(3 * time.Second), LastActive: now.Add(-30 * time.Minute)},
		{ID: "recent-match", ProjectDir: "/proj", CreatedAt: p2.Add(-24 * time.Hour), LastActive: now.Add(-3 * time.Minute)},
	}

	MarkActiveSessions(sessions, mkActive("/proj", p1, p2))

	if !sessions[0].IsActive {
		t.Error("created-match should be active via phase 1")
	}
	if !sessions[1].IsActive {
		t.Error("recent-match should be active via phase 2")
	}
}
