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

## 2026-03-18 — Mission Control Rework Discussion

### Multi-follow / tiled dashboard mode
Discussed during brainstorming: a 2x2 (or auto-layout) grid showing live pane capture for all active sessions simultaneously. Like follow mode but for all sessions at once. Would be the ultimate "mission control" view. Deferred because single-follow + expanded active rows cover most of the need, but worth revisiting if monitoring 4+ sessions becomes common.

### Session lifecycle events / activity feed
A chronological log of cross-session events: "12:34 piclaw started", "12:36 investiq committed: Fix i18n", "12:38 ccs error in go test". Shows what happened while you were focused elsewhere. Deferred — the expanded active rows with live status partially solve this, but a timeline view would add value for longer monitoring sessions.

### tmux window alert integration
When a session needs attention (permission prompt, error, completion), set the tmux window to "alert" state so the ccs tab highlights even when not focused. Lightweight integration — just a tmux bell/activity flag. Deferred because attention states in the TUI are the first priority, but this would complement them for the "not looking at ccs" case.

### Stale session nudges
Sessions in "open" state that haven't been touched in N days get a subtle visual indicator suggesting they might be done. Not auto-completing — just a hint like dimming or a "stale?" badge. Keeps the open list honest without being aggressive. Deferred — wait to see how the manual complete workflow feels first.

### CLI quick-launch mode
`ccs piclaw` or `ccs piclaw "fix auth"` — fuzzy-match project and launch directly without opening the TUI. Not a priority (the value of ccs is monitoring, not launching faster), but would be a nice convenience layer once the core rework is done.

### Frecency-sorted projects in search
When `/` search shows project directories, sort by frecency (frequency + recency) rather than alphabetical. Projects worked on daily float to top. Low priority since search is fuzzy anyway, but would improve the "I just need to start a new session" flow.

### Auto-naming prompt iteration
The haiku naming prompt will need tuning based on real-world results. The initial prompt is task-oriented ("what is this session accomplishing?") but may need refinement. Key insight from discussion: Claude's own `/rename` grabs "interesting" details instead of the actual task — e.g., naming a config-sync setup session "bash-set-e-footgun-fix" because it latched onto a footnote. The ccs prompt must explicitly focus on the goal/task, not incidental details.
