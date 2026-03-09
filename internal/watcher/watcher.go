package watcher

import (
	"ccs/internal/activity"
	"log"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ActivityUpdate is sent when a watched JSONL file is modified.
type ActivityUpdate struct {
	SessionID string
	FilePath  string
	Entries   []activity.Entry
}

// watchedFile tracks a session ID associated with a file path.
type watchedFile struct {
	sessionID string
}

// Watcher monitors JSONL session files for changes and sends parsed
// activity updates on a channel.
type Watcher struct {
	fw            *fsnotify.Watcher
	watched       map[string]watchedFile // filePath → watchedFile
	mu            sync.Mutex
	updates       chan ActivityUpdate
	activityLines int
	done          chan struct{}
	stopped       chan struct{} // closed by Run() when it exits
	closeOnce     sync.Once
}

// New creates a Watcher that will parse the last activityLines entries
// from modified files.
func New(activityLines int) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		fw:            fw,
		watched:       make(map[string]watchedFile),
		updates:       make(chan ActivityUpdate, 100),
		activityLines: activityLines,
		done:          make(chan struct{}),
		stopped:       make(chan struct{}),
	}, nil
}

// Watch starts monitoring a file path for the given session ID.
func (w *Watcher) Watch(sessionID, filePath string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.watched[filePath]; exists {
		// Already watching — update session ID if different.
		w.watched[filePath] = watchedFile{sessionID: sessionID}
		return nil
	}

	if err := w.fw.Add(filePath); err != nil {
		return err
	}
	w.watched[filePath] = watchedFile{sessionID: sessionID}
	return nil
}

// Unwatch stops monitoring a file path.
func (w *Watcher) Unwatch(filePath string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.watched[filePath]; !exists {
		return
	}
	_ = w.fw.Remove(filePath)
	delete(w.watched, filePath)
}

// UnwatchAll removes all watched file paths.
func (w *Watcher) UnwatchAll() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for fp := range w.watched {
		_ = w.fw.Remove(fp)
	}
	w.watched = make(map[string]watchedFile)
}

// Updates returns a read-only channel of ActivityUpdate messages.
// The TUI reads from this channel to receive file change notifications.
func (w *Watcher) Updates() <-chan ActivityUpdate {
	return w.updates
}

// Run processes fsnotify events, debouncing writes per file path.
// It should be called in a goroutine.
func (w *Watcher) Run() {
	defer close(w.stopped)

	const debounceInterval = 200 * time.Millisecond

	// Per-file debounce timers.
	timers := make(map[string]*time.Timer)

	for {
		select {
		case <-w.done:
			// Drain timers.
			for _, t := range timers {
				t.Stop()
			}
			return

		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write) {
				continue
			}

			filePath := event.Name

			w.mu.Lock()
			wf, exists := w.watched[filePath]
			lines := w.activityLines
			w.mu.Unlock()

			if !exists {
				continue
			}

			sid := wf.sessionID

			// Debounce: reset timer for this file path.
			if t, ok := timers[filePath]; ok {
				t.Stop()
			}
			timers[filePath] = time.AfterFunc(debounceInterval, func() {
				entries := activity.TailFile(filePath, lines)
				update := ActivityUpdate{
					SessionID: sid,
					FilePath:  filePath,
					Entries:   entries,
				}
				select {
				case w.updates <- update:
				case <-w.done:
				}
			})

		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

// Close shuts down the watcher and closes the updates channel.
func (w *Watcher) Close() {
	w.closeOnce.Do(func() {
		close(w.done)
		_ = w.fw.Close()
		// Wait for Run() to exit before closing the updates channel.
		// Run() is the only sender, so this is safe once it has stopped.
		go func() {
			<-w.stopped
			close(w.updates)
		}()
	})
}
