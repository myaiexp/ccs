package session

import (
	"testing"
	"time"

	"ccs/internal/types"
)

func TestMarkActive_SingleProcess(t *testing.T) {
	now := time.Now()
	sessions := []types.Session{
		{ID: "recent", ProjectDir: "/proj", LastActive: now},
		{ID: "middle", ProjectDir: "/proj", LastActive: now.Add(-1 * time.Hour)},
		{ID: "old", ProjectDir: "/proj", LastActive: now.Add(-2 * time.Hour)},
	}
	active := types.ActiveInfo{
		ProjectDirs: map[string]types.ProjectActiveInfo{
			"/proj": {Count: 1, EarliestStart: now.Add(-3 * time.Hour)},
		},
	}

	MarkActiveSessions(sessions, active)

	if !sessions[0].IsActive {
		t.Error("expected most recent session to be active")
	}
	if sessions[1].IsActive || sessions[2].IsActive {
		t.Error("expected only one session to be active")
	}
}

func TestMarkActive_MultipleProcesses(t *testing.T) {
	now := time.Now()
	sessions := []types.Session{
		{ID: "s1", ProjectDir: "/proj", LastActive: now},
		{ID: "s2", ProjectDir: "/proj", LastActive: now.Add(-10 * time.Minute)},
		{ID: "s3", ProjectDir: "/proj", LastActive: now.Add(-20 * time.Minute)},
		{ID: "s4", ProjectDir: "/proj", LastActive: now.Add(-30 * time.Minute)},
		{ID: "s5", ProjectDir: "/proj", LastActive: now.Add(-40 * time.Minute)},
	}
	active := types.ActiveInfo{
		ProjectDirs: map[string]types.ProjectActiveInfo{
			"/proj": {Count: 3, EarliestStart: now.Add(-1 * time.Hour)},
		},
	}

	MarkActiveSessions(sessions, active)

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
	processStart := now.Add(-1 * time.Hour)
	sessions := []types.Session{
		{ID: "new", ProjectDir: "/proj", LastActive: now},
		{ID: "old", ProjectDir: "/proj", LastActive: processStart.Add(-1 * time.Minute)},
	}
	active := types.ActiveInfo{
		ProjectDirs: map[string]types.ProjectActiveInfo{
			"/proj": {Count: 2, EarliestStart: processStart},
		},
	}

	MarkActiveSessions(sessions, active)

	if !sessions[0].IsActive {
		t.Error("new session should be active")
	}
	if sessions[1].IsActive {
		t.Error("old session (before process start) should not be active")
	}
}

func TestMarkActive_MultipleProjects(t *testing.T) {
	now := time.Now()
	sessions := []types.Session{
		{ID: "a1", ProjectDir: "/projA", LastActive: now},
		{ID: "a2", ProjectDir: "/projA", LastActive: now.Add(-10 * time.Minute)},
		{ID: "b1", ProjectDir: "/projB", LastActive: now},
		{ID: "b2", ProjectDir: "/projB", LastActive: now.Add(-10 * time.Minute)},
	}
	active := types.ActiveInfo{
		ProjectDirs: map[string]types.ProjectActiveInfo{
			"/projA": {Count: 1, EarliestStart: now.Add(-1 * time.Hour)},
			"/projB": {Count: 2, EarliestStart: now.Add(-1 * time.Hour)},
		},
	}

	MarkActiveSessions(sessions, active)

	if !sessions[0].IsActive {
		t.Error("a1 should be active (1 process in projA)")
	}
	if sessions[1].IsActive {
		t.Error("a2 should not be active")
	}
	if !sessions[2].IsActive || !sessions[3].IsActive {
		t.Error("both projB sessions should be active (2 processes)")
	}
}

func TestMarkActive_NoProcesses(t *testing.T) {
	sessions := []types.Session{
		{ID: "s1", ProjectDir: "/proj", LastActive: time.Now()},
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
	sessions := []types.Session{
		{ID: "s1", ProjectDir: "/proj", LastActive: now},
		{ID: "s2", ProjectDir: "/proj", LastActive: now.Add(-5 * time.Minute)},
	}
	active := types.ActiveInfo{
		ProjectDirs: map[string]types.ProjectActiveInfo{
			"/proj": {Count: 5, EarliestStart: now.Add(-1 * time.Hour)},
		},
	}

	MarkActiveSessions(sessions, active)

	if !sessions[0].IsActive || !sessions[1].IsActive {
		t.Error("both qualifying sessions should be active")
	}
}
