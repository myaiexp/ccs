package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcher_WatchUnwatch(t *testing.T) {
	w, err := New(5)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	// Create a temp file to watch.
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "test.jsonl")
	if err := os.WriteFile(fp, []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Watch it.
	if err := w.Watch("sess-1", fp); err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	w.mu.Lock()
	if _, exists := w.watched[fp]; !exists {
		t.Error("expected file to be in watched map after Watch()")
	}
	w.mu.Unlock()

	// Watch same file again with different session — should update, not error.
	if err := w.Watch("sess-2", fp); err != nil {
		t.Fatalf("Watch() second call error: %v", err)
	}
	w.mu.Lock()
	if w.watched[fp].sessionID != "sess-2" {
		t.Error("expected session ID to be updated on re-watch")
	}
	w.mu.Unlock()

	// Unwatch.
	w.Unwatch(fp)
	w.mu.Lock()
	if _, exists := w.watched[fp]; exists {
		t.Error("expected file to be removed from watched map after Unwatch()")
	}
	w.mu.Unlock()

	// Unwatch non-existent — should not panic.
	w.Unwatch("/nonexistent")

	// UnwatchAll.
	fp2 := filepath.Join(tmp, "test2.jsonl")
	if err := os.WriteFile(fp2, []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_ = w.Watch("sess-3", fp)
	_ = w.Watch("sess-4", fp2)
	w.UnwatchAll()

	w.mu.Lock()
	if len(w.watched) != 0 {
		t.Errorf("expected empty watched map after UnwatchAll(), got %d entries", len(w.watched))
	}
	w.mu.Unlock()
}

func TestWatcher_FileModification(t *testing.T) {
	w, err := New(5)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	// Start the event loop.
	go w.Run()

	// Create a temp JSONL file with valid content.
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "session.jsonl")

	initialContent := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}]},"timestamp":"2026-03-08T10:00:00Z"}` + "\n"
	if err := os.WriteFile(fp, []byte(initialContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Watch the file.
	if err := w.Watch("sess-test", fp); err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	// Give fsnotify a moment to register the watch.
	time.Sleep(50 * time.Millisecond)

	// Append a new line to trigger a write event.
	f, err := os.OpenFile(fp, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	newLine := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"go test"}}]},"timestamp":"2026-03-08T10:01:00Z"}` + "\n"
	if _, err := f.WriteString(newLine); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	f.Close()

	// Wait for debounced update (200ms debounce + some buffer).
	select {
	case update := <-w.Updates():
		if update.SessionID != "sess-test" {
			t.Errorf("expected session ID 'sess-test', got %q", update.SessionID)
		}
		if update.FilePath != fp {
			t.Errorf("expected file path %q, got %q", fp, update.FilePath)
		}
		if len(update.Entries) == 0 {
			t.Error("expected at least one activity entry")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ActivityUpdate")
	}
}
