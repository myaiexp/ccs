package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionState holds the lifecycle state and ccs-owned metadata for a session.
type SessionState struct {
	Status      string     `json:"status"`       // "open" or "done"
	Name        string     `json:"name"`
	NameSource  string     `json:"name_source"`  // "auto" or "manual"
	CompletedAt *time.Time `json:"completed_at"`
}

// stateFile is the on-disk JSON format.
type stateFile struct {
	Sessions map[string]SessionState `json:"sessions"`
}

// Store manages session lifecycle state, persisted to disk.
type Store struct {
	mu       sync.Mutex
	sessions map[string]SessionState
	path     string
}

// Load loads the state store from the default path (~/.cache/ccs/state.json).
func Load() *Store {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = filepath.Join(os.TempDir(), "ccs")
	}
	return loadFromPath(filepath.Join(dir, "ccs", "state.json"))
}

// LoadFromDir loads state from a specific directory (for testing).
func LoadFromDir(dir string) *Store {
	return loadFromPath(filepath.Join(dir, "state.json"))
}

// loadFromPath loads state from a specific path.
func loadFromPath(path string) *Store {
	s := &Store{
		sessions: make(map[string]SessionState),
		path:     path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}

	var f stateFile
	if err := json.Unmarshal(data, &f); err != nil {
		return s
	}
	if f.Sessions != nil {
		s.sessions = f.Sessions
	}
	return s
}

// Get returns the state for a session ID.
func (s *Store) Get(id string) (SessionState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[id]
	return st, ok
}

// Has returns whether a session ID exists in the store.
func (s *Store) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.sessions[id]
	return ok
}

// MarkOpen sets a session's status to "open". Creates the entry if missing.
func (s *Store) MarkOpen(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.sessions[id]
	st.Status = "open"
	s.sessions[id] = st
	s.save()
}

// MarkDone sets a session's status to "done" and records CompletedAt.
// Caller should check that the session is not active before calling.
func (s *Store) MarkDone(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.sessions[id]
	st.Status = "done"
	now := time.Now()
	st.CompletedAt = &now
	s.sessions[id] = st
	s.save()
}

// Reopen moves a done session back to open, clearing CompletedAt.
func (s *Store) Reopen(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.sessions[id]
	if !ok {
		return
	}
	st.Status = "open"
	st.CompletedAt = nil
	s.sessions[id] = st
	s.save()
}

// SetName sets the display name for a session.
// If existing source is "manual" and new source is "auto", this is a no-op.
func (s *Store) SetName(id, name, source string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.sessions[id]
	if st.NameSource == "manual" && source == "auto" {
		return
	}
	st.Name = name
	st.NameSource = source
	s.sessions[id] = st
	s.save()
}

// Remove deletes a session entry from the store.
func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, id)
	s.save()
}

func (s *Store) save() {
	_ = os.MkdirAll(filepath.Dir(s.path), 0755)
	f := stateFile{Sessions: s.sessions}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, data, 0644)
}
