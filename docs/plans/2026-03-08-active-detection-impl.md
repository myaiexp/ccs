# Fix Active Session Detection — Implementation Plan

**Goal:** Replace broken active session detection (relies on unused `--resume` flag + unreliable single-session-per-project fallback) with count-based mtime matching that correctly handles multiple concurrent sessions per project.

**Architecture:** Scan `/proc` for claude processes, collect per-project-dir process counts and earliest start times. For each project dir with N processes, mark the N most recently modified sessions (with mtime >= earliest start) as active. Deduplicate the marking logic (currently copy-pasted in `main.go` and `tui/model.go`) into a shared function.

**Tech Stack:** Go, `/proc` filesystem

---

### Task 1: Update Types and Detection [Mode: Direct]

**Files:**
- Modify: `internal/types/types.go`
- Modify: `internal/session/detect.go`
- Test: `internal/session/detect_test.go` (create)

**Contracts:**

```go
// types.go — replace ActiveInfo
type ProjectActiveInfo struct {
    Count        int       // number of running claude processes
    EarliestStart time.Time // earliest process start time
}

type ActiveInfo struct {
    ProjectDirs map[string]ProjectActiveInfo // key: absolute project dir path
}
```

```go
// detect.go — updated DetectActive
func DetectActive() types.ActiveInfo
// Returns ActiveInfo with ProjectDirs keyed by ABSOLUTE path (not encoded).
// Each entry has the count of claude processes and the earliest start time.
// Process start time read from /proc/<pid> stat (file mod time of /proc/<pid>).
```

Key changes in `DetectActive()`:
- Read process start time via `stat /proc/<pid>` (the directory's mtime = process start)
- Decode the cwd to absolute path immediately (using filepath, not `encodePathToProjectDir`)
- Accumulate count and track earliest start per absolute project dir
- Drop `--resume` / `SessionIDs` parsing entirely

**Test Cases:**

```go
func TestDetectActive_ReturnsEmptyOnNoProcesses(t *testing.T) {
    // Just verify the function returns without panic and has initialized maps
    info := DetectActive()
    if info.ProjectDirs == nil {
        t.Error("ProjectDirs should be initialized")
    }
}
```

(Live process detection is hard to unit test — integration-tested via manual verification. The marking logic in Task 2 is where the testable logic lives.)

**Verification:**
Run: `go build ./... && go test ./internal/session/ -v -run TestDetect`

**Commit after passing.**

---

### Task 2: Rewrite Marking Logic as Shared Function [Mode: Direct]

**Files:**
- Create: `internal/session/mark.go`
- Create: `internal/session/mark_test.go`
- Modify: `main.go` (remove `markActiveSessions`, call shared function)
- Modify: `internal/tui/model.go` (remove duplicated marking, call shared function)

**Contracts:**

```go
// mark.go
func MarkActiveSessions(sessions []types.Session, active types.ActiveInfo)
// Mutates sessions in place, setting IsActive = true for active ones.
//
// Algorithm for each project dir in active.ProjectDirs:
//   1. Collect sessions matching that project dir
//   2. Filter to those with LastActive (file mtime) >= active.ProjectDirs[dir].EarliestStart
//   3. Sort by LastActive descending
//   4. Mark top N as active (N = active.ProjectDirs[dir].Count)
```

**Test Cases:**

```go
func TestMarkActive_SingleProcess(t *testing.T) {
    // 1 process in /proj, 3 sessions — only the most recent (by LastActive) gets marked
}

func TestMarkActive_MultipleProcesses(t *testing.T) {
    // 3 processes in /proj, 5 sessions — top 3 by LastActive (after earliest start) marked
}

func TestMarkActive_FiltersOldSessions(t *testing.T) {
    // Sessions with LastActive before process start time are NOT marked
}

func TestMarkActive_MultipleProjects(t *testing.T) {
    // 2 different project dirs, each with different process counts — correct N per project
}

func TestMarkActive_NoProcesses(t *testing.T) {
    // Empty ActiveInfo — no sessions marked active
}

func TestMarkActive_MoreProcessesThanSessions(t *testing.T) {
    // 3 processes but only 2 qualifying sessions — both marked, no panic
}
```

**Constraints:**
- Sessions are pre-sorted by `LastActive` desc from `DiscoverSessions`, but don't rely on that — sort the filtered subset explicitly
- Use `session.LastActive` (populated from file mtime in `parse.go` line 156) for comparison

**Verification:**
Run: `go test ./internal/session/ -v -run TestMark`

**Commit after passing.**

---

### Task 3: Update DiscoverProjects and Wire Everything [Mode: Direct]

**Files:**
- Modify: `internal/project/project.go` (update `DiscoverProjects` signature — drop unused `active` param)
- Modify: `internal/project/project_test.go` (update call sites)
- Modify: `main.go` (wire new flow)
- Modify: `internal/tui/model.go` (wire new flow in `handleRefresh`)

**Contracts:**

`DiscoverProjects` currently takes `active types.ActiveInfo` but only uses it indirectly through `s.IsActive` (which is already set by marking). The `active` parameter is unused — remove it.

```go
// project.go — simplified signature
func DiscoverProjects(sessions []types.Session, cfg *types.Config) []types.Project
```

Main flow becomes:
```go
sessions := session.DiscoverSessions(projectsDir)
active := session.DetectActive()
session.MarkActiveSessions(sessions, active)
projects := project.DiscoverProjects(sessions, cfg)
```

Same pattern in `handleRefresh()`.

**Verification:**
Run: `go test ./... -count=1 && go build -o /dev/null .`

**Commit after passing.**

---

### Task 4: Build, Install, Manual Verification [Mode: Direct]

**Files:**
- None created/modified

**Verification:**
1. `go build -o ~/.local/bin/ccs .`
2. Launch ccs, verify:
   - Sessions with running claude processes show active indicator (green dot)
   - Multiple sessions in same project (e.g. poe-crafting) all show active
   - Sessions without running processes show inactive
   - After exiting a claude session and returning to ccs, the indicator updates

**Commit: not needed (build artifact only).**

---

## Execution
**Skill:** superpowers:subagent-driven-development
- Mode A tasks: Opus implements directly (all tasks)
- Mode B tasks: None
