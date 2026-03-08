package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"ccs/internal/types"
)

// TrackedSession represents a session launched from ccs.
type TrackedSession struct {
	SessionID  string    `json:"session_id,omitempty"` // empty for new sessions
	ProjectDir string    `json:"project_dir"`
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"started_at"`
}

// Tracker manages the state of sessions launched from ccs.
type Tracker struct {
	mu       sync.Mutex
	Sessions []TrackedSession `json:"sessions"`
	path     string
}

func trackerPath() string {
	dir, _ := os.UserCacheDir()
	return filepath.Join(dir, "ccs", "active.json")
}

// LoadTracker loads the tracker state from disk, pruning dead PIDs.
func LoadTracker() *Tracker {
	t := &Tracker{
		path: trackerPath(),
	}

	data, err := os.ReadFile(t.path)
	if err == nil {
		_ = json.Unmarshal(data, t)
	}

	t.prune()
	return t
}

// Track records a launched session.
func (t *Tracker) Track(sessionID, projectDir string, pid int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Sessions = append(t.Sessions, TrackedSession{
		SessionID:  sessionID,
		ProjectDir: projectDir,
		PID:        pid,
		StartedAt:  time.Now(),
	})
	t.save()
}

// Refresh prunes dead PIDs and seeds from /proc for --resume sessions
// not already tracked.
func (t *Tracker) Refresh() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.prune()

	// Scan /proc for claude processes with --resume that we're not tracking
	tracked := make(map[int]bool)
	for _, s := range t.Sessions {
		tracked[s.PID] = true
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.save()
		return
	}

	selfPID := os.Getpid()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == selfPID {
			continue
		}
		if tracked[pid] {
			continue
		}

		cmdlineBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(string(cmdlineBytes), "\x00")
		if len(args) == 0 || filepath.Base(args[0]) != "claude" {
			continue
		}

		cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if err != nil {
			continue
		}

		procStat, err := os.Stat(filepath.Join("/proc", entry.Name()))
		if err != nil {
			continue
		}

		// Check for --resume flag
		var sessionID string
		for i, arg := range args {
			if arg == "--resume" && i+1 < len(args) && args[i+1] != "" {
				sessionID = args[i+1]
				break
			}
		}

		t.Sessions = append(t.Sessions, TrackedSession{
			SessionID:  sessionID,
			ProjectDir: cwd,
			PID:        pid,
			StartedAt:  procStat.ModTime(),
		})
	}

	t.save()
}

// OpenSessionIDs returns the set of session IDs that are currently open.
func (t *Tracker) OpenSessionIDs() map[string]bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	ids := make(map[string]bool)
	for _, s := range t.Sessions {
		if s.SessionID != "" {
			ids[s.SessionID] = true
		}
	}
	return ids
}

// OpenProjectDirs returns project dirs that have open processes
// (including those without a session ID yet).
func (t *Tracker) OpenProjectDirs() map[string]bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	dirs := make(map[string]bool)
	for _, s := range t.Sessions {
		dirs[s.ProjectDir] = true
	}
	return dirs
}

// MatchNewSession tries to match a tracked entry (PID with no session ID)
// to a session that was created after the process started in the same project dir.
func (t *Tracker) MatchNewSession(sessions []types.Session) {
	t.mu.Lock()
	defer t.mu.Unlock()

	changed := false
	for i := range t.Sessions {
		if t.Sessions[i].SessionID != "" {
			continue
		}
		// Find the session created closest after this process started
		var bestIdx int = -1
		var bestGap time.Duration = 2 * time.Minute

		for j := range sessions {
			if sessions[j].ProjectDir != t.Sessions[i].ProjectDir {
				continue
			}
			if sessions[j].CreatedAt.IsZero() {
				continue
			}
			gap := sessions[j].CreatedAt.Sub(t.Sessions[i].StartedAt)
			if gap < -10*time.Second || gap > 2*time.Minute {
				continue
			}
			if gap < 0 {
				gap = -gap
			}
			if gap < bestGap {
				bestGap = gap
				bestIdx = j
			}
		}

		if bestIdx >= 0 {
			t.Sessions[i].SessionID = sessions[bestIdx].ID
			changed = true
		}
	}

	if changed {
		t.save()
	}
}

// prune removes entries for processes that are no longer running.
func (t *Tracker) prune() {
	alive := t.Sessions[:0]
	for _, s := range t.Sessions {
		if processAlive(s.PID) {
			alive = append(alive, s)
		}
	}
	t.Sessions = alive
}

func processAlive(pid int) bool {
	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
	return err == nil
}

func (t *Tracker) save() {
	_ = os.MkdirAll(filepath.Dir(t.path), 0755)
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(t.path, data, 0644)
}
