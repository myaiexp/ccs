# Fix Active Session Detection

## Problem

Active session detection is broken. Two failure modes:
1. **False negatives**: Multiple claude processes in the same project dir → only one session marked active
2. **False positives**: Heuristic picks wrong session (most recent by mtime, not necessarily the running one)

Root cause: current approach relies on `--resume` flag in process cmdline (never present in practice) and falls back to "most recent session in project dir" (unreliable with multiple processes).

## Solution: Count-Based Mtime Matching

For each project directory with running claude processes:

1. Count N = number of running claude processes with that cwd
2. Get each process's start time from `/proc/<pid>` stat
3. Find sessions in that project dir whose file mtime >= earliest process start time
4. Sort matching sessions by mtime descending
5. Mark the top N as active

### Why This Works

- Claude creates/writes session JSONL files during interactions → active sessions have recent mtimes
- The N most recently modified sessions in a project dir correspond to the N running processes
- Sessions from previous (dead) processes have mtimes before current processes started → filtered out
- Handles compaction: new session file gets the latest mtime, naturally becomes the "most recent"
- Handles idle processes: session file was still created/modified during process lifetime

### Verified Against Real Data

6 running claude processes across 4 project dirs. Algorithm correctly identifies all active sessions, including 3 concurrent sessions in poe-crafting.

## Changes

### `internal/session/detect.go`

Replace current `DetectActive()`:
- Return per-project-dir process counts and earliest start times (not just boolean maps)
- Drop `SessionIDs` map (--resume parsing) — unused in practice

### `internal/types/types.go`

Update `ActiveInfo` struct:
- `ProjectDirs map[string]ProjectActiveInfo` where `ProjectActiveInfo` has `Count int` and `EarliestStart time.Time`

### `main.go` + `internal/tui/model.go`

Update `markActiveSessions()`:
- For each project dir in ActiveInfo, filter sessions by mtime >= earliest start, take top N by mtime
- Remove the two-pass approach (exact ID match + fallback)
