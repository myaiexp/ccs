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

	ids := tracker.ActiveSessionIDs()
	if !ids["sess-1"] || !ids["sess-2"] {
		t.Error("expected both session IDs to be open")
	}
}

func TestTracker_PruneDeadPIDs(t *testing.T) {
	tracker := &Tracker{path: "/dev/null"}
	tracker.Track("alive", "/proj", os.Getpid())
	tracker.Track("dead", "/proj", 999999999)

	tracker.Refresh()

	ids := tracker.ActiveSessionIDs()
	if !ids["alive"] {
		t.Error("alive session should still be tracked")
	}
	if ids["dead"] {
		t.Error("dead session should be pruned")
	}
}

func TestTracker_ActiveProjectDirs(t *testing.T) {
	tracker := &Tracker{path: "/dev/null"}
	tracker.Track("", "/proj/a", os.Getpid())
	tracker.Track("sess-1", "/proj/b", os.Getpid())

	dirs := tracker.ActiveProjectDirs()
	if !dirs["/proj/a"] || !dirs["/proj/b"] {
		t.Error("expected both project dirs to be open")
	}
}

func TestTracker_FindBySessionID(t *testing.T) {
	tracker := &Tracker{path: "/dev/null"}
	tracker.Track("sess-1", "/proj/a", os.Getpid())
	tracker.Track("sess-2", "/proj/b", os.Getpid())

	ts, ok := tracker.FindBySessionID("sess-1")
	if !ok {
		t.Fatal("expected to find sess-1")
	}
	if ts.ProjectDir != "/proj/a" {
		t.Errorf("expected /proj/a, got %s", ts.ProjectDir)
	}

	_, ok = tracker.FindBySessionID("nonexistent")
	if ok {
		t.Error("should not find nonexistent session")
	}
}

func TestTracker_TmuxWindowIDs(t *testing.T) {
	tracker := &Tracker{path: "/dev/null"}
	tracker.Track("sess-1", "/proj/a", os.Getpid())
	tracker.Track("sess-2", "/proj/b", os.Getpid())
	tracker.SetTmuxWindow("sess-1", "@1")

	ids := tracker.TmuxWindowIDs()
	if ids["sess-1"] != "@1" {
		t.Errorf("expected @1 for sess-1, got %q", ids["sess-1"])
	}
	if _, ok := ids["sess-2"]; ok {
		t.Error("sess-2 should not have a tmux window ID")
	}
}

func TestTracker_SetTmuxWindow(t *testing.T) {
	tracker := &Tracker{path: "/dev/null"}
	tracker.Track("sess-1", "/proj/a", os.Getpid())
	tracker.SetTmuxWindow("sess-1", "@5")

	ts, ok := tracker.FindBySessionID("sess-1")
	if !ok {
		t.Fatal("expected to find sess-1")
	}
	if ts.TmuxWindowID != "@5" {
		t.Errorf("expected @5, got %q", ts.TmuxWindowID)
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

func TestTracker_DetectSessionSwitch(t *testing.T) {
	// Set up a temp dir simulating ~/.claude/projects/{encoded-dir}/
	dir := t.TempDir()

	// Create "old" JSONL file with stale mtime (60s ago)
	oldFile := dir + "/old-session.jsonl"
	os.WriteFile(oldFile, []byte(`{"type":"user"}`), 0644)
	staleTime := time.Now().Add(-10 * time.Second)
	os.Chtimes(oldFile, staleTime, staleTime)

	// Create "new" JSONL file with recent mtime
	newFile := dir + "/new-session.jsonl"
	os.WriteFile(newFile, []byte(`{"type":"user"}`), 0644)

	tracker := &Tracker{path: "/dev/null"}
	tracker.Sessions = []TrackedSession{
		{SessionID: "old-session", ProjectDir: "/proj", PID: os.Getpid()},
	}

	sessions := []types.Session{
		{ID: "old-session", FilePath: oldFile},
	}

	tracker.DetectSessionSwitch(sessions)

	if tracker.Sessions[0].SessionID != "" {
		t.Errorf("expected session ID cleared, got %q", tracker.Sessions[0].SessionID)
	}
}

func TestTracker_DetectSessionSwitch_RecentNotCleared(t *testing.T) {
	// If the JSONL was modified recently, don't clear even if newer files exist
	dir := t.TempDir()

	oldFile := dir + "/active-session.jsonl"
	os.WriteFile(oldFile, []byte(`{"type":"user"}`), 0644)
	// mtime is now (recent) — don't touch it

	newFile := dir + "/other-session.jsonl"
	os.WriteFile(newFile, []byte(`{"type":"user"}`), 0644)

	tracker := &Tracker{path: "/dev/null"}
	tracker.Sessions = []TrackedSession{
		{SessionID: "active-session", ProjectDir: "/proj", PID: os.Getpid()},
	}

	sessions := []types.Session{
		{ID: "active-session", FilePath: oldFile},
	}

	tracker.DetectSessionSwitch(sessions)

	if tracker.Sessions[0].SessionID != "active-session" {
		t.Error("should NOT clear session ID when JSONL was recently modified")
	}
}

func TestTracker_DetectSessionSwitch_NoNewerFile(t *testing.T) {
	// Stale JSONL but no newer file — don't clear (Claude just thinking)
	dir := t.TempDir()

	oldFile := dir + "/only-session.jsonl"
	os.WriteFile(oldFile, []byte(`{"type":"user"}`), 0644)
	staleTime := time.Now().Add(-10 * time.Second)
	os.Chtimes(oldFile, staleTime, staleTime)

	tracker := &Tracker{path: "/dev/null"}
	tracker.Sessions = []TrackedSession{
		{SessionID: "only-session", ProjectDir: "/proj", PID: os.Getpid()},
	}

	sessions := []types.Session{
		{ID: "only-session", FilePath: oldFile},
	}

	tracker.DetectSessionSwitch(sessions)

	if tracker.Sessions[0].SessionID != "only-session" {
		t.Error("should NOT clear when no newer JSONL exists")
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
