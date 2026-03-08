package session

import (
	"os"
	"testing"
	"time"

	"ccs/internal/types"
)

func TestTracker_TrackAndOpenIDs(t *testing.T) {
	tracker := &Tracker{path: "/dev/null"}
	tracker.Track("sess-1", "/proj/a", os.Getpid())
	tracker.Track("sess-2", "/proj/b", os.Getpid())

	ids := tracker.OpenSessionIDs()
	if !ids["sess-1"] || !ids["sess-2"] {
		t.Error("expected both session IDs to be open")
	}
}

func TestTracker_PruneDeadPIDs(t *testing.T) {
	tracker := &Tracker{path: "/dev/null"}
	tracker.Track("alive", "/proj", os.Getpid())
	tracker.Track("dead", "/proj", 999999999)

	tracker.Refresh()

	ids := tracker.OpenSessionIDs()
	if !ids["alive"] {
		t.Error("alive session should still be tracked")
	}
	if ids["dead"] {
		t.Error("dead session should be pruned")
	}
}

func TestTracker_OpenProjectDirs(t *testing.T) {
	tracker := &Tracker{path: "/dev/null"}
	tracker.Track("", "/proj/a", os.Getpid())
	tracker.Track("sess-1", "/proj/b", os.Getpid())

	dirs := tracker.OpenProjectDirs()
	if !dirs["/proj/a"] || !dirs["/proj/b"] {
		t.Error("expected both project dirs to be open")
	}
}

func TestTracker_MatchNewSession(t *testing.T) {
	now := time.Now()
	tracker := &Tracker{path: "/dev/null"}
	tracker.Sessions = []TrackedSession{
		{ProjectDir: "/proj", PID: os.Getpid(), StartedAt: now.Add(-30 * time.Second)},
	}

	sessions := []types.Session{
		{ID: "found-it", ProjectDir: "/proj", CreatedAt: now.Add(-28 * time.Second)},
		{ID: "too-old", ProjectDir: "/proj", CreatedAt: now.Add(-5 * time.Minute)},
	}

	tracker.MatchNewSession(sessions)

	if tracker.Sessions[0].SessionID != "found-it" {
		t.Errorf("expected session ID 'found-it', got %q", tracker.Sessions[0].SessionID)
	}
}

func TestTracker_MatchNewSession_CreatedBeforeProcess(t *testing.T) {
	now := time.Now()
	tracker := &Tracker{path: "/dev/null"}
	tracker.Sessions = []TrackedSession{
		{ProjectDir: "/proj", PID: os.Getpid(), StartedAt: now.Add(-30 * time.Second)},
	}

	sessions := []types.Session{
		{ID: "before-start", ProjectDir: "/proj", CreatedAt: now.Add(-5 * time.Minute)},
	}

	tracker.MatchNewSession(sessions)

	if tracker.Sessions[0].SessionID != "" {
		t.Error("should not match session created before process started")
	}
}

func TestTracker_MatchNewSession_LateCreation(t *testing.T) {
	// Simulates the ccs case: process started at 09:23, session created at 11:02
	now := time.Now()
	tracker := &Tracker{path: "/dev/null"}
	tracker.Sessions = []TrackedSession{
		{ProjectDir: "/proj", PID: os.Getpid(), StartedAt: now.Add(-2 * time.Hour)},
	}

	sessions := []types.Session{
		{ID: "late-session", ProjectDir: "/proj", CreatedAt: now.Add(-20 * time.Minute)},
		{ID: "old-session", ProjectDir: "/proj", CreatedAt: now.Add(-3 * time.Hour)},
	}

	tracker.MatchNewSession(sessions)

	if tracker.Sessions[0].SessionID != "late-session" {
		t.Errorf("expected 'late-session', got %q", tracker.Sessions[0].SessionID)
	}
}

func TestTracker_MatchNewSession_MostRecentWins(t *testing.T) {
	// Multiple sessions created after process start — most recent wins
	now := time.Now()
	tracker := &Tracker{path: "/dev/null"}
	tracker.Sessions = []TrackedSession{
		{ProjectDir: "/proj", PID: os.Getpid(), StartedAt: now.Add(-2 * time.Hour)},
	}

	sessions := []types.Session{
		{ID: "older", ProjectDir: "/proj", CreatedAt: now.Add(-90 * time.Minute)},
		{ID: "newest", ProjectDir: "/proj", CreatedAt: now.Add(-30 * time.Minute)},
		{ID: "middle", ProjectDir: "/proj", CreatedAt: now.Add(-60 * time.Minute)},
	}

	tracker.MatchNewSession(sessions)

	if tracker.Sessions[0].SessionID != "newest" {
		t.Errorf("expected 'newest' (most recent), got %q", tracker.Sessions[0].SessionID)
	}
}

func TestTracker_MatchNewSession_NoDoubleClaim(t *testing.T) {
	// Two tracked processes — each should get a different session
	now := time.Now()
	tracker := &Tracker{path: "/dev/null"}
	tracker.Sessions = []TrackedSession{
		{ProjectDir: "/proj", PID: os.Getpid(), StartedAt: now.Add(-2 * time.Hour)},
		{ProjectDir: "/proj", PID: os.Getpid(), StartedAt: now.Add(-1 * time.Hour)},
	}

	sessions := []types.Session{
		{ID: "sess-a", ProjectDir: "/proj", CreatedAt: now.Add(-110 * time.Minute)}, // after 1st process, before 2nd
		{ID: "sess-b", ProjectDir: "/proj", CreatedAt: now.Add(-30 * time.Minute)},  // after both
	}

	tracker.MatchNewSession(sessions)

	ids := make(map[string]bool)
	for _, ts := range tracker.Sessions {
		if ts.SessionID != "" {
			ids[ts.SessionID] = true
		}
	}

	if len(ids) != 2 {
		t.Errorf("expected 2 unique session IDs, got %d", len(ids))
	}
}
