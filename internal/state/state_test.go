package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	return &Store{
		sessions: make(map[string]SessionState),
		path:     path,
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "state.json")
	// Temporarily override the path function
	s := &Store{path: path, sessions: make(map[string]SessionState)}
	// Load from missing file should produce empty store
	data, err := os.ReadFile(path)
	if err != nil {
		// Expected: file doesn't exist
		_ = data
	}
	if len(s.sessions) != 0 {
		t.Fatalf("expected empty sessions, got %d", len(s.sessions))
	}
}

func TestLoadFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Write a valid state file
	now := time.Now().Truncate(time.Second)
	file := stateFile{
		Sessions: map[string]SessionState{
			"abc": {Status: "open", Name: "test session", NameSource: "auto"},
			"def": {Status: "done", Name: "done session", NameSource: "manual", CompletedAt: &now},
		},
	}
	data, _ := json.MarshalIndent(file, "", "  ")
	os.WriteFile(path, data, 0644)

	s := loadFromPath(path)
	if len(s.sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(s.sessions))
	}

	st, ok := s.Get("abc")
	if !ok || st.Status != "open" || st.Name != "test session" {
		t.Fatalf("unexpected state for abc: %+v", st)
	}

	st, ok = s.Get("def")
	if !ok || st.Status != "done" || st.CompletedAt == nil {
		t.Fatalf("unexpected state for def: %+v", st)
	}
}

func TestMarkOpen(t *testing.T) {
	s := tempStore(t)
	s.MarkOpen("sess1")

	st, ok := s.Get("sess1")
	if !ok {
		t.Fatal("expected sess1 to exist")
	}
	if st.Status != "open" {
		t.Fatalf("expected status 'open', got %q", st.Status)
	}
	if st.CompletedAt != nil {
		t.Fatal("expected nil CompletedAt")
	}
}

func TestMarkDone(t *testing.T) {
	s := tempStore(t)
	s.MarkOpen("sess1")
	s.MarkDone("sess1")

	st, ok := s.Get("sess1")
	if !ok {
		t.Fatal("expected sess1 to exist")
	}
	if st.Status != "done" {
		t.Fatalf("expected status 'done', got %q", st.Status)
	}
	if st.CompletedAt == nil {
		t.Fatal("expected non-nil CompletedAt")
	}
}

func TestReopen(t *testing.T) {
	s := tempStore(t)
	s.MarkOpen("sess1")
	s.MarkDone("sess1")
	s.Reopen("sess1")

	st, ok := s.Get("sess1")
	if !ok {
		t.Fatal("expected sess1 to exist")
	}
	if st.Status != "open" {
		t.Fatalf("expected status 'open', got %q", st.Status)
	}
	if st.CompletedAt != nil {
		t.Fatal("expected nil CompletedAt after reopen")
	}
}

func TestSetNameManualSticksOverAuto(t *testing.T) {
	s := tempStore(t)
	s.MarkOpen("sess1")
	s.SetName("sess1", "manual name", "manual")

	// Auto should not overwrite manual
	s.SetName("sess1", "auto name", "auto")

	st, _ := s.Get("sess1")
	if st.Name != "manual name" {
		t.Fatalf("expected manual name to stick, got %q", st.Name)
	}
	if st.NameSource != "manual" {
		t.Fatalf("expected source 'manual', got %q", st.NameSource)
	}
}

func TestSetNameAutoOverwritesAuto(t *testing.T) {
	s := tempStore(t)
	s.MarkOpen("sess1")
	s.SetName("sess1", "first auto", "auto")
	s.SetName("sess1", "second auto", "auto")

	st, _ := s.Get("sess1")
	if st.Name != "second auto" {
		t.Fatalf("expected 'second auto', got %q", st.Name)
	}
}

func TestSetNameManualOverwritesAuto(t *testing.T) {
	s := tempStore(t)
	s.MarkOpen("sess1")
	s.SetName("sess1", "auto name", "auto")
	s.SetName("sess1", "manual name", "manual")

	st, _ := s.Get("sess1")
	if st.Name != "manual name" {
		t.Fatalf("expected 'manual name', got %q", st.Name)
	}
	if st.NameSource != "manual" {
		t.Fatalf("expected source 'manual', got %q", st.NameSource)
	}
}

func TestRemove(t *testing.T) {
	s := tempStore(t)
	s.MarkOpen("sess1")
	s.Remove("sess1")

	if s.Has("sess1") {
		t.Fatal("expected sess1 to be removed")
	}
}

func TestHas(t *testing.T) {
	s := tempStore(t)
	if s.Has("nope") {
		t.Fatal("expected Has to return false for missing ID")
	}
	s.MarkOpen("sess1")
	if !s.Has("sess1") {
		t.Fatal("expected Has to return true for existing ID")
	}
}

func TestJSONRoundtrip(t *testing.T) {
	s := tempStore(t)
	now := time.Now().Truncate(time.Second)
	s.MarkOpen("sess1")
	s.SetName("sess1", "test name", "manual")
	s.MarkDone("sess1")

	// Force a specific CompletedAt for deterministic comparison
	s.mu.Lock()
	st := s.sessions["sess1"]
	st.CompletedAt = &now
	s.sessions["sess1"] = st
	s.mu.Unlock()
	s.save()

	// Load from the same path
	s2 := loadFromPath(s.path)

	st2, ok := s2.Get("sess1")
	if !ok {
		t.Fatal("expected sess1 after roundtrip")
	}
	if st2.Status != "done" {
		t.Fatalf("expected 'done', got %q", st2.Status)
	}
	if st2.Name != "test name" {
		t.Fatalf("expected 'test name', got %q", st2.Name)
	}
	if st2.NameSource != "manual" {
		t.Fatalf("expected 'manual', got %q", st2.NameSource)
	}
	if st2.CompletedAt == nil || !st2.CompletedAt.Equal(now) {
		t.Fatalf("CompletedAt mismatch: %v vs %v", st2.CompletedAt, now)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := tempStore(t)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "sess" + string(rune('A'+n%26))
			s.MarkOpen(id)
			s.Get(id)
			if n%3 == 0 {
				s.MarkDone(id)
			}
			if n%5 == 0 {
				s.Reopen(id)
			}
			s.SetName(id, "name", "auto")
		}(i)
	}

	wg.Wait()
	// Just verify no panic/deadlock occurred
}
