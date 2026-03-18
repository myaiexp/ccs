package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ccs/internal/tmux"
	"ccs/internal/types"
)

// TrackedSession represents a session launched from ccs.
type TrackedSession struct {
	SessionID    string    `json:"session_id,omitempty"` // empty for new sessions
	ProjectDir   string    `json:"project_dir"`
	PID          int       `json:"pid"`
	StartedAt    time.Time `json:"started_at"`
	TmuxWindowID string    `json:"tmux_window_id,omitempty"`
}

// Tracker manages the state of sessions launched from ccs.
type Tracker struct {
	mu       sync.Mutex
	Sessions []TrackedSession `json:"sessions"`
	path     string
}

func trackerPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "ccs", "active.json")
	}
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

	// Match PIDs to tmux windows for sessions missing a window ID
	panePIDs, _ := tmux.PanePIDs()
	for i := range t.Sessions {
		if t.Sessions[i].TmuxWindowID == "" {
			if wid, ok := panePIDs[t.Sessions[i].PID]; ok {
				t.Sessions[i].TmuxWindowID = wid
			}
		}
	}

	t.save()
}

// ActiveSessionIDs returns the set of session IDs that are currently active.
func (t *Tracker) ActiveSessionIDs() map[string]bool {
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

// ActiveProjectDirs returns project dirs that have active processes
// (including those without a session ID yet).
func (t *Tracker) ActiveProjectDirs() map[string]bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	dirs := make(map[string]bool)
	for _, s := range t.Sessions {
		dirs[s.ProjectDir] = true
	}
	return dirs
}

// FindBySessionID returns the tracked session with the given ID, if any.
func (t *Tracker) FindBySessionID(id string) (TrackedSession, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, s := range t.Sessions {
		if s.SessionID == id {
			return s, true
		}
	}
	return TrackedSession{}, false
}

// TmuxWindowIDs returns a map of session ID → tmux window ID
// for all tracked sessions that have a tmux window.
func (t *Tracker) TmuxWindowIDs() map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()

	ids := make(map[string]string)
	for _, s := range t.Sessions {
		if s.SessionID != "" && s.TmuxWindowID != "" {
			ids[s.SessionID] = s.TmuxWindowID
		}
	}
	return ids
}

// MarkActive sets ActiveSource on sessions based on tracker state.
func (t *Tracker) MarkActive(sessions []types.Session) {
	openIDs := t.ActiveSessionIDs()
	tmuxWindows := t.TmuxWindowIDs()
	for i := range sessions {
		if openIDs[sessions[i].ID] {
			if _, hasTmux := tmuxWindows[sessions[i].ID]; hasTmux {
				sessions[i].ActiveSource = types.SourceTmux
			} else {
				sessions[i].ActiveSource = types.SourceProc
			}
		}
	}
}

// SetTmuxWindow sets the tmux window ID for a tracked session.
func (t *Tracker) SetTmuxWindow(sessionID, windowID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range t.Sessions {
		if t.Sessions[i].SessionID == sessionID {
			t.Sessions[i].TmuxWindowID = windowID
			t.save()
			return
		}
	}
}

// MatchNewSession tries to match tracked entries (PIDs without session IDs)
// to sessions. For each unmatched tracked entry, finds the most recently
// created session in the same project dir that was created after the process
// started. Already-claimed session IDs are not reused.
func (t *Tracker) MatchNewSession(sessions []types.Session) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Collect session IDs already claimed by tracked entries
	claimed := make(map[string]bool)
	for _, ts := range t.Sessions {
		if ts.SessionID != "" {
			claimed[ts.SessionID] = true
		}
	}

	// Group unmatched tracker entries by project dir
	byDir := make(map[string][]int)
	for i := range t.Sessions {
		if t.Sessions[i].SessionID == "" {
			byDir[t.Sessions[i].ProjectDir] = append(byDir[t.Sessions[i].ProjectDir], i)
		}
	}

	changed := false
	for dir, indices := range byDir {
		// Collect unclaimed sessions created after the earliest process in this dir
		var earliest time.Time
		for _, i := range indices {
			if earliest.IsZero() || t.Sessions[i].StartedAt.Before(earliest) {
				earliest = t.Sessions[i].StartedAt
			}
		}

		var candidates []int
		for j := range sessions {
			if sessions[j].ProjectDir != dir || claimed[sessions[j].ID] || sessions[j].CreatedAt.IsZero() {
				continue
			}
			if sessions[j].CreatedAt.Before(earliest.Add(-10 * time.Second)) {
				continue
			}
			candidates = append(candidates, j)
		}

		if len(indices) == 1 {
			// Single process: pick most recently created session (handles compaction)
			var bestIdx int = -1
			var bestCreated time.Time
			for _, j := range candidates {
				if bestIdx == -1 || sessions[j].CreatedAt.After(bestCreated) {
					bestCreated = sessions[j].CreatedAt
					bestIdx = j
				}
			}
			if bestIdx >= 0 {
				t.Sessions[indices[0]].SessionID = sessions[bestIdx].ID
				claimed[sessions[bestIdx].ID] = true
				changed = true
			}
		} else {
			// Multiple processes: sort both by time ascending and match 1:1
			sort.Slice(indices, func(a, b int) bool {
				return t.Sessions[indices[a]].StartedAt.Before(t.Sessions[indices[b]].StartedAt)
			})
			sort.Slice(candidates, func(a, b int) bool {
				return sessions[candidates[a]].CreatedAt.Before(sessions[candidates[b]].CreatedAt)
			})

			ci := 0
			for _, ti := range indices {
				// Find the first unclaimed candidate created after this process started
				for ci < len(candidates) {
					j := candidates[ci]
					ci++
					if sessions[j].CreatedAt.Before(t.Sessions[ti].StartedAt.Add(-10 * time.Second)) {
						continue
					}
					t.Sessions[ti].SessionID = sessions[j].ID
					claimed[sessions[j].ID] = true
					changed = true
					break
				}
			}
		}
	}

	if changed {
		t.save()
	}
}

// prune removes entries for processes that are no longer running
// and clears tmux window IDs for windows that no longer exist.
func (t *Tracker) prune() {
	alive := t.Sessions[:0]
	for _, s := range t.Sessions {
		if processAlive(s.PID) {
			// Clear tmux window ID if the window no longer exists
			if s.TmuxWindowID != "" && !tmux.WindowExists(s.TmuxWindowID) {
				s.TmuxWindowID = ""
			}
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
