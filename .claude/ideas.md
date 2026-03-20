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

### Pane capture reliability investigation
Active sessions frequently show `with_pane=0` in the naming log. The tracker has tmux window IDs for sessions launched via ccs, but sessions started manually (`cd && claude`) may not get tracked properly. Need to investigate: why pane content disappears after initial capture, whether the tracker's PID→tmux mapping is breaking on refresh, and whether there's a race between pane capture tick and session refresh.

### Color-coded attention states
DeriveStatus already detects waiting/permission/thinking/error states, but they all render in the same gray italic. Should be color-coded: bright yellow for "waiting for input", orange for "permission prompt", red for errors, green for "working". This would make the dashboard actually useful for spotting which session needs you.

### In-progress AI summary for open sessions
Currently the comprehensive summary only generates on transition (active → open). Open sessions that were active before ccs started have no summary. Could generate a summary retroactively from JSONL conversation text when an open session is selected and has no summary yet.

## 2026-03-20 — Tmux Integration: Brainstorm Exhaustively

CCS already lives inside tmux and uses it for launching/switching sessions. This idea is about going much deeper — making tmux the primary event bus for CCS reactivity, replacing polling with tmux hooks and leveraging tmux features that are currently untapped.

**Brainstorm this topic completely, exhaustively, thoroughly.** Cover every tmux feature, hook, and capability that could improve CCS. Consider: hooks, control mode, formats, alerts, signals, layout management, popup/overlay windows, custom key tables, environment variables, status line integration, and anything else tmux offers.

Areas to explore (non-exhaustive starting points):

### Hooks for event-driven reactivity
- `window-linked` / `window-unlinked` — instant detection of session windows appearing/disappearing
- `pane-died` — instant PID death detection (replaces /proc polling)
- `pane-focus-in` / `pane-focus-out` — know which session the user is looking at
- `window-renamed` — detect if claude renames its window
- `alert-activity` / `alert-bell` / `alert-silence` — activity/inactivity signals from session panes
- `session-window-changed` — user switched tabs, update "currently viewing" state
- `after-resize-pane` / `after-resize-window` — react to layout changes
- Custom hooks via `set-hook` — CCS could register hooks on startup and deregister on exit

### Control mode (`tmux -C`)
- Persistent connection to tmux server via stdin/stdout
- Receives all events as structured text (no polling needed)
- Can send commands and get responses synchronously
- Could replace all tmux CLI calls with a single persistent connection
- Eliminates fork/exec overhead of shelling out to `tmux` repeatedly

### Alerts and monitoring
- `monitor-activity` / `monitor-silence` — tmux-native activity detection per window
- `activity-action` / `silence-action` — auto-trigger actions on activity/silence
- Bell forwarding — session errors could ring the bell, CCS detects via hook
- `visual-activity` / `visual-bell` / `visual-silence` — visual indicators in tmux status

### Layout and window management
- Named windows with structured naming convention (e.g., `ccs:projectname`)
- Window reordering to keep active sessions grouped
- Automatic layout adjustment when sessions start/stop
- Split panes for side-by-side session monitoring within CCS
- `select-layout` for auto-tiling multiple follow views

### Popup and overlay windows
- `display-popup` — overlay windows for quick session info/actions without leaving current pane
- Popup for session detail view (summary, activity, context %)
- Popup for quick-launch menu (project selector)
- Popup for attention alerts ("session X needs input")

### Status line integration
- CCS could set tmux status-right with live session counts/status
- Per-window status showing session state (active/waiting/error)
- Format strings with `#{pane_current_command}` for live process info
- Color-coded status based on attention states

### Environment variables and session metadata
- `tmux setenv` to pass CCS metadata to session panes
- `tmux showenv` to read session state without file I/O
- Could replace `active.json` tracker with tmux environment state
- Session-to-window mapping stored in tmux instead of on disk

### Key tables and input handling
- Custom key table for CCS-specific bindings that work from any pane
- Prefix-free shortcuts for common CCS actions (switch session, follow, etc.)
- `send-keys` for programmatic input to sessions (e.g., auto-approve permissions)

### Pane capture improvements
- `capture-pane -p -t` with explicit target for reliable capture
- `pipe-pane` — stream pane output to a file/pipe continuously (replaces polling)
- `capture-pane -e` for ANSI-aware capture (preserve colors)
- `copy-mode` integration for scrollback access

### Process and pane introspection
- `list-panes -F` with format strings for structured pane data
- `#{pane_pid}`, `#{pane_current_command}`, `#{pane_start_command}` — direct PID/command info
- `#{window_activity}` — last activity timestamp per window (replaces mtime checks)
- `#{pane_dead}` — instant dead pane detection

### Session lifecycle via tmux
- `wait-for` channels — tmux-native IPC between CCS and session panes
- `run-shell` hooks that notify CCS of events
- Respawn dead panes for session restart
- `remain-on-exit` for post-mortem inspection of crashed sessions

### Auto-naming prompt iteration
The haiku naming prompt will need tuning based on real-world results. The initial prompt is task-oriented ("what is this session accomplishing?") but may need refinement. Key insight from discussion: Claude's own `/rename` grabs "interesting" details instead of the actual task — e.g., naming a config-sync setup session "bash-set-e-footgun-fix" because it latched onto a footnote. The ccs prompt must explicitly focus on the goal/task, not incidental details.
