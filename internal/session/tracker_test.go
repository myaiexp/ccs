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
	tracker.Track("dead", "/proj", 999999999) // very unlikely to be a real PID

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
	tracker.Track("", "/proj/a", os.Getpid()) // new session, no ID yet
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

func TestTracker_MatchNewSession_OutsideWindow(t *testing.T) {
	now := time.Now()
	tracker := &Tracker{path: "/dev/null"}
	tracker.Sessions = []TrackedSession{
		{ProjectDir: "/proj", PID: os.Getpid(), StartedAt: now.Add(-30 * time.Second)},
	}

	sessions := []types.Session{
		{ID: "too-late", ProjectDir: "/proj", CreatedAt: now.Add(3 * time.Minute)},
	}

	tracker.MatchNewSession(sessions)

	if tracker.Sessions[0].SessionID != "" {
		t.Error("should not match session outside 2-minute window")
	}
}
