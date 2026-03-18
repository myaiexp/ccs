# ccs — Ideas

## Pinned Sessions

Pin important sessions so they stay at the top of the list regardless of recency. Useful for long-running project sessions you return to repeatedly — keeps them accessible even as newer throwaway sessions push them down.

Possible approach:
- `p` key to toggle pin on selected session
- Store pinned session IDs in `~/.config/ccs/config.toml` (e.g. `pinned_sessions = ["abc123", "def456"]`)
- Pinned sessions render above the regular list with a pin indicator (📌 or `*`)
- Still sorted by last active within the pinned group

## Save/Restore Active Sessions

Snapshot which sessions are currently active (running) and save them as a "workspace buffer." Later, restore by launching all saved sessions at once. Single buffer — saving overwrites the previous snapshot.

Possible approach:
- `S` key to save current active sessions (stores session IDs + project dirs in config)
- `L` key to restore/launch all saved sessions
- Config: `saved_sessions = [{id = "abc123", project_dir = "/home/mse/Projects/foo"}, ...]`
- Visual indicator in the UI showing saved sessions exist (e.g., statusbar note)
- Depends on active session detection working correctly first

## Tracker Mutex Safety

`LoadTracker()` calls `json.Unmarshal(data, t)` which writes to `t.Sessions` without holding the mutex. Safe today because it's only called at startup before concurrent access, but fragile — adding a goroutine that touches the tracker earlier in the lifecycle would create a data race. Fix: hold the lock during unmarshal, or unmarshal into a local and assign under lock.

## Embedded Session View (tmux popup/overlay)

Hide the ccs tmux window and embed a live session view directly in the ccs TUI — follow what a session is doing without switching tabs. Could use tmux's `capture-pane` to mirror the session output into a ccs pane, or a tmux popup/overlay. Would let you monitor a session's full terminal output from within ccs without occupying a separate tab.
