# CCS Tabbed Session Wrapper — Design

> Transform ccs from a session monitor into a tabbed session manager where Claude sessions live as tmux windows managed by ccs, with a unified tab bar and attention system.

## Core Concept

ccs becomes the **wrapper** for Claude Code sessions, not a sidecar monitor. All sessions run as tmux windows within the ccs tmux session. ccs takes over the tmux status line to render a smart tab bar with attention states. The dashboard is just another window (window 0) you switch to and from.

## Architecture

### Multiple windows in a managed tmux session

Each Claude session gets its own tmux window within the ccs tmux session. ccs runs in window 0 (the dashboard). Tab switching = `tmux select-window`, which is instant and flicker-free.

- **Dashboard window** (window 0) — runs the bubbletea TUI. The "home tab."
- **Session windows** — one per Claude session. Created by ccs via `tmux new-window`, or adopted from other locations via `tmux move-window`.

This maps directly to tmux's native model. No pane zoom hacks, no visual flicker on tab switch.

### Session lifecycle

1. **Launch** — user picks a project from dashboard or uses `ccs launch`. ccs creates a window with `tmux new-window -t <session> "claude [flags]"`, sets `@ccs-managed` on it, registers the window ID.
2. **Adopt** — ccs detects Claude processes in tmux windows outside the ccs session. Uses `tmux move-window -s <window-id> -t <ccs-session>: -d` to pull them in (trailing colon auto-assigns next available index, `-d` avoids focus switch during adoption). Sets `@ccs-managed` on the adopted window. Adoption is automatic, no confirmation.
3. **Active** — session running, window exists, attention state monitored via pane capture.
4. **Inactive** — Claude process exits. Session moves to "open" state in dashboard. Can be resumed (creates new window).
5. **Done** — user marks it done from dashboard.

### Stateless window management

ccs is stateless with respect to windows. If it crashes or is restarted, it rediscovers all Claude windows on startup and re-registers them. No session is lost. Session windows continue running independently of ccs.

## Tab Bar (tmux status line)

ccs takes over the tmux status line on startup and restores it on exit. Requires tmux 3.4+ (user has 3.6a).

### Line 1 — tabs

```
 ⌂  │  piclaw: auth flow  │ ▸ ccs: tab rework  │  investiq ●  │  poe-crafting
```

- `⌂` = dashboard, always first (window 0)
- `▸` = currently focused window
- `●` = attention needed (colored: yellow = waiting for input, red = error, orange = permission prompt)
- Tab names = project name + haiku-generated short name
- Overflow: `+N more` indicator when tabs exceed terminal width

### Line 2 — attention summary (dynamic)

```
 piclaw waiting for input · investiq error in tests
```

- Only appears when at least one session needs attention
- Disappears when all sessions are working — line collapses, vertical space returned to session
- Key value: see which sessions need you without leaving the current one

### Status line update mechanism

ccs periodically updates the tmux status line via `tmux set-option status-format[0]` and `status-format[1]`. The bubbletea process drives updates (on tick, on attention state change, on tab switch detection). ccs detects which window is active by polling `tmux display-message -p '#{window_id}'` on each tick to keep the tab bar highlight current.

## Keybindings

Three bindings registered on startup via `tmux bind-key`. Each uses `if-shell` to check the `@ccs-managed` window option — only fires in the ccs session, falls through to the user's original binding otherwise.

**Scoping mechanism:** `if-shell "tmux show -wv @ccs-managed 2>/dev/null"` (no `-q` flag — `-q` forces exit 0 regardless, breaking the conditional). Without `-q`, `show` returns exit 1 when the option is unset.

**Fallback capture:** On startup, ccs queries `tmux list-keys -T prefix` for Space, 1, and 2 to capture the user's current bindings. These become the `if-shell` else branch, ensuring non-ccs windows retain the user's actual bindings (not tmux defaults).

```
bind-key -T prefix Space if-shell "tmux show -wv @ccs-managed 2>/dev/null" "select-window -t :0" "<original-space-binding>"
bind-key -T prefix 1     if-shell "tmux show -wv @ccs-managed 2>/dev/null" "previous-window"     "<original-1-binding>"
bind-key -T prefix 2     if-shell "tmux show -wv @ccs-managed 2>/dev/null" "next-window"          "<original-2-binding>"
```

On shutdown, ccs restores the original bindings directly. On abnormal exit, the `if-shell` fallback ensures non-ccs windows still work correctly.

## Dashboard Tab

The existing bubbletea TUI, simplified since the tab bar handles monitoring:

### Kept

- Session list (open + done sections, j/k navigation)
- Search (`/`) — fuzzy match sessions and project dirs, enter to launch or switch
- Session detail pane: left column = AI comprehensive summary, right column = conversation text
- Sort, rename, delete, mark done/reopen
- AI status summaries (haiku-generated, shown in detail pane and tab bar)

### Changed

- `enter` on active session → `select-window` to its tab
- `enter` on inactive session → creates new window with `claude --resume`, switches to it
- `enter` on project dir in search → creates new window with `claude`, switches to it
- Follow mode (`f`) removed — switching to the tab IS following the session
- Number shortcuts (`1-9`) removed from dashboard — tmux keybindings handle tab switching
- Active section simplified — list with attention badges, no expanded multi-line status rows (tab bar covers that)
- Status summaries still shown in detail pane when a session is selected

## `ccs launch` CLI

Universal interface for starting managed sessions — used by the dashboard UI, Kelo, scripts, automation.

### Usage

```
ccs launch [--project <dir>] [--resume <id>] [--prompt <text>] [--on-done <command>]
```

### IPC

Running ccs instance listens on a unix socket at `~/.cache/ccs/ccs.sock`.

- `ccs launch` connects to the socket and sends the launch command as JSON.
- If the socket doesn't exist or connection fails, `ccs launch` errors with "ccs is not running" (no auto-start — avoids the bootstrapping race condition with `syscall.Exec`).
- Stale socket handling: on startup, ccs attempts `net.Listen("unix", path)`. If it fails (socket in use by a live process), ccs exits with "another instance is running." If the socket file exists but no one is listening (stale), ccs removes it and creates a new one. No separate PID file needed — the socket itself is the lock.
- `--prompt` is passed as the positional prompt argument to `claude`, i.e., `tmux new-window ... "claude [flags] <prompt>"`.

### `--on-done` callback

When the session completes, ccs runs the callback command. Session exit is detected via `set-hook -w pane-exited` registered on each managed window — tmux fires this hook when the pane's process exits, and ccs listens for it (the hook sends a notification to ccs via the unix socket). ccs then waits for the haiku transition summary to complete before firing the callback, so the summary is available.

Environment variables passed to the callback:
- `CCS_SESSION_ID` — the session ID
- `CCS_SESSION_PROJECT` — project directory
- `CCS_SESSION_SUMMARY` — haiku-generated summary of what was accomplished

If the summary generation times out (10s), the callback fires anyway with an empty summary.

### Examples

```bash
# Kelo from VPS:
ssh desktop "ccs launch --project ~/Projects/piclaw --prompt 'audit tracker docs' --on-done 'ssh vps kelo-callback \$CCS_SESSION_ID'"

# Local quick-launch:
ccs launch --project ~/Projects/ccs

# Resume:
ccs launch --resume abc123
```

## Startup & Shutdown

### Startup

1. Check tmux — bootstrap into a new tmux session if not already inside tmux
2. Clean up stale socket/pid files if they exist
3. Open unix socket at `~/.cache/ccs/ccs.sock`, write PID to `~/.cache/ccs/ccs.pid`
4. Set `@ccs-managed` window option on all windows in the session
5. Replace tmux status line with ccs tab bar using session-scoped options (`set-option -s`) — no save/restore needed, `set-option -su` (unset) on shutdown reverts to global defaults
6. Capture original keybindings for Space/1/2 via `tmux list-keys -T prefix`, register scoped bindings with captured originals as fallbacks
7. Scan for Claude processes in other tmux windows → auto-adopt via `move-window`
8. Start bubbletea dashboard in window 0
9. Begin monitoring (JSONL watching, attention detection, AI summaries, status line updates)

### Shutdown

1. Unset session-scoped status line options (`set-option -su`) — reverts to global defaults
2. Restore original keybindings captured at startup
3. Close unix socket (file auto-removed by listener close)
4. **Session windows keep running** — ccs exiting doesn't kill Claude sessions
5. Next ccs startup re-discovers and re-registers orphaned windows

## Deferred Ideas

- **Persistent sidebar/header (option C)** — dashboard info always visible alongside the active session tab. Natural evolution once the tab system works.
- **Multi-follow / tiled dashboard** — 2x2 grid showing multiple session panes simultaneously.
