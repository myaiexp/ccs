# ccs — Claude Code Session Hub Implementation Plan

**Goal:** Build a Go TUI that serves as the entry point for Claude Code — listing recent sessions, discovering projects, and launching/resuming claude sessions.

**Architecture:** Bubbletea TUI with two-section layout (sessions list + projects grid). Reads `~/.claude/projects/` JSONL files for session data, detects active sessions via `/proc`. Uses `tea.ExecProcess` to suspend the TUI while claude runs, resuming when it exits (persistent wrapper for free). Fuzzy search via `sahilm/fuzzy`.

**Tech Stack:** Go, bubbletea v2, lipgloss, bubbles (textinput), sahilm/fuzzy, BurntSushi/toml

---

### Task 1: Project Scaffolding & Types

**Files:**
- Create: `go.mod`
- Create: `internal/types/types.go`
- Create: `main.go` (minimal, just starts tea.Program)

**Contracts:**
```go
// internal/types/types.go
type Session struct {
    ID          string    // Full UUID
    ShortID     string    // First 8 chars
    ProjectName string    // Decoded project name (e.g. "poe-proof", "~")
    ProjectDir  string    // Absolute path (e.g. "/home/mse/Projects/poe-proof")
    Title       string    // Session title or first message
    ContextPct  int       // 0-100
    MsgCount    int       // Number of user messages
    LastActive  time.Time // JSONL mtime
    IsActive    bool      // Currently has a running claude process
    FilePath    string    // Path to JSONL file
}

type Project struct {
    Name       string    // Display name
    Dir        string    // Absolute path
    LastActive time.Time // Most recent session mtime
    HasActive  bool      // Has a running claude process
    Hidden     bool      // Hidden in config
}

type Config struct {
    HiddenProjects []string // Project names to hide
    ClaudeFlags    []string // Extra flags to pass to claude
}
```

**Constraints:**
- Module path: `github.com/mase/ccs` (or just `ccs` — local only)
- Go 1.24+

**Verification:**
Run: `cd ~/Projects/ccs && go build .`
Expected: Compiles, runs, shows blank TUI, exits on q

**Commit after passing.**

[Mode: Direct]

---

### Task 2: Config Loading

**Files:**
- Create: `internal/config/config.go`

**Contracts:**
```go
// Load reads ~/.config/ccs/config.toml, returns defaults if missing
func Load() (*types.Config, error)
```

**Test Cases:**
```go
func TestLoadDefaults(t *testing.T) {
    // When config file doesn't exist, returns default config
    cfg, err := Load()
    assert.NoError(t, err)
    assert.Empty(t, cfg.HiddenProjects)
}

func TestLoadFromFile(t *testing.T) {
    // Parse a valid TOML with hidden_projects and claude_flags
}
```

**Config format:**
```toml
hidden_projects = ["cloned", ".claude"]
claude_flags = ["--dangerously-skip-permissions"]
```

**Verification:**
Run: `go test ./internal/config/ -v`

**Commit after passing.**

[Mode: Direct]

---

### Task 3: JSONL Parsing & Session Discovery

**Files:**
- Create: `internal/session/parse.go`
- Create: `internal/session/parse_test.go`

**Contracts:**
```go
// ParseSession reads a JSONL file and extracts session metadata.
// Does NOT read full content — only enough for listing.
func ParseSession(filepath string) (*types.Session, error)

// DiscoverSessions finds all session JSONL files in ~/.claude/projects/,
// parses them, and returns sorted by LastActive descending.
// Skips files in subagents/ dirs and files < 25KB.
func DiscoverSessions(projectsDir string) ([]types.Session, error)

// DecodeProjectDir converts encoded dir name to display name and absolute path.
// "-home-mse-Projects-poe-proof" → ("poe-proof", "/home/mse/Projects/poe-proof")
// "-home-mse" → ("~", "/home/mse")
// "-home-mse--openclaw" → (".openclaw", "/home/mse/.openclaw")
func DecodeProjectDir(encoded string) (name string, absPath string)
```

**Test Cases:**
```go
func TestDecodeProjectDir(t *testing.T) {
    // Test all encoding patterns from cc-sessions
    name, path := DecodeProjectDir("-home-mse-Projects-poe-proof")
    assert.Equal(t, "poe-proof", name)
    assert.Equal(t, "/home/mse/Projects/poe-proof", path)

    name, path = DecodeProjectDir("-home-mse")
    assert.Equal(t, "~", name)

    name, path = DecodeProjectDir("-home-mse--openclaw")
    assert.Equal(t, ".openclaw", name)
}

func TestParseSessionTitle(t *testing.T) {
    // Renamed session extracts title from "Session renamed to: ..."
    // Unnamed session uses first user message
    // Empty session returns "(untitled)"
}

func TestParseContextPercent(t *testing.T) {
    // Calculates from last assistant message usage tokens
}
```

**Constraints:**
- Title extraction priority: renamed title > first non-meta user message > "(untitled)"
- Context % = (input_tokens + cache_creation_input_tokens + cache_read_input_tokens) * 100 / 200000
- Read file line-by-line (streaming), don't load entire file into memory
- For context %, read file backwards (last assistant message) — can read last 50 lines

**Verification:**
Run: `go test ./internal/session/ -v`

**Commit after passing.**

[Mode: Delegated]

---

### Task 4: Active Session Detection

**Files:**
- Create: `internal/session/detect.go`
- Create: `internal/session/detect_test.go`

**Contracts:**
```go
// DetectActive returns a map of project directory paths that have
// a running claude process (keyed by encoded project dir name).
// Works by reading /proc/<pid>/cwd for all claude processes.
func DetectActive() map[string]bool
```

**Implementation approach:**
1. Find PIDs: read `/proc/*/cmdline`, match processes containing `claude` (the binary, not subprocesses)
2. For each PID, `readlink /proc/<pid>/cwd`
3. Encode the cwd path (`/` → `-`) to get the project dir key
4. Return set of active project dirs

**Test Cases:**
```go
func TestEncodePathToProjectDir(t *testing.T) {
    assert.Equal(t, "-home-mse-Projects-foo", encodePathToProjectDir("/home/mse/Projects/foo"))
    assert.Equal(t, "-home-mse", encodePathToProjectDir("/home/mse"))
}
```

**Constraints:**
- Linux-only (/proc filesystem)
- Must not fail if /proc is unreadable — just return empty map
- Filter for actual claude binary processes, not grep/editor etc.

**Verification:**
Run: `go test ./internal/session/ -v`

**Commit after passing.**

[Mode: Direct]

---

### Task 5: Project Discovery

**Files:**
- Create: `internal/project/project.go`
- Create: `internal/project/project_test.go`

**Contracts:**
```go
// DiscoverProjects extracts unique projects from session data and
// the filesystem. Merges with config for hidden status.
func DiscoverProjects(sessions []types.Session, activeDirs map[string]bool, cfg *types.Config) []types.Project
```

**Logic:**
1. Collect unique projects from sessions (already have name, dir, last active)
2. Mark active based on `activeDirs`
3. Mark hidden based on `cfg.HiddenProjects`
4. Sort: active first, then by last active descending

**Test Cases:**
```go
func TestDiscoverProjects(t *testing.T) {
    // Deduplicates projects from sessions
    // Marks active correctly
    // Hides configured projects
    // Sorts active first, then by recency
}
```

**Verification:**
Run: `go test ./internal/project/ -v`

**Commit after passing.**

[Mode: Direct]

---

### Task 6: TUI Model & Main View

**Files:**
- Create: `internal/tui/model.go`
- Create: `internal/tui/styles.go`
- Create: `internal/tui/keys.go`
- Modify: `main.go`

**Contracts:**
```go
// internal/tui/model.go
type Model struct {
    sessions     []types.Session
    projects     []types.Project
    config       *types.Config
    filter       textinput.Model  // bubbles textinput
    focus        Focus            // Sessions or Projects
    sessionIdx   int              // selected session index
    projectIdx   int              // selected project index
    filtering    bool             // filter bar active
    showHidden   bool             // show hidden projects
    width        int              // terminal width
    height       int              // terminal height
}

type Focus int
const (
    FocusSessions Focus = iota
    FocusProjects
)

func New(sessions []types.Session, projects []types.Project, cfg *types.Config) Model
func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m Model) View() string
```

**View layout:**
- Header: "ccs" + filter bar
- Sessions section: scrollable list with status indicator, number [1-9], project, title, ctx%, time
- Projects section: horizontal wrapping grid with project names
- Footer: keybinding hints

**Key handling:**
- `1-9`: resume session by number
- `enter`: resume selected session / new in selected project
- `n`: jump to projects section
- `/`: activate filter
- `tab`: toggle focus
- `d`: delete session (confirm)
- `h`: toggle hidden projects
- `?`: help
- `q`/`ctrl+c`: quit
- `j/k`/arrows: navigate

**Constraints:**
- Fuzzy filter (sahilm/fuzzy) applies to both sessions (title+project) and projects (name)
- Number shortcuts [1-9] always map to the currently visible (filtered) top 9 sessions
- Responsive: adapt to terminal width

**Verification:**
Run: `cd ~/Projects/ccs && go build . && ./ccs`
Expected: Shows sessions and projects, navigation works, filter works

**Commit after passing.**

[Mode: Delegated]

---

### Task 7: Session Launch (ExecProcess)

**Files:**
- Modify: `internal/tui/model.go` (add launch logic to Update)
- Create: `internal/tui/launch.go`

**Contracts:**
```go
// LaunchResume returns a tea.Cmd that execs into claude --resume <id>
// in the correct project directory. TUI suspends and resumes on exit.
func LaunchResume(session types.Session, flags []string) tea.Cmd

// LaunchNew returns a tea.Cmd that execs into claude in the given
// project directory. TUI suspends and resumes on exit.
func LaunchNew(project types.Project, flags []string) tea.Cmd
```

**Flow:**
1. Build `exec.Cmd` with claude binary + flags + --resume (if resuming)
2. Set `cmd.Dir` to project directory
3. Return `tea.ExecProcess(cmd, callback)`
4. On callback: refresh session list (re-parse JSONLs), redraw

**Constraints:**
- Must pass through `config.ClaudeFlags` (e.g. `--dangerously-skip-permissions`)
- Claude binary path: use `exec.LookPath("claude")` to find it
- After claude exits, refresh the session/project data before redrawing

**Verification:**
Run: Build and test manually — resume a session, verify TUI comes back after exit
Expected: claude launches, TUI suspends, TUI resumes with updated data after claude exits

**Commit after passing.**

[Mode: Delegated]

---

### Task 8: Session Deletion & Polish

**Files:**
- Modify: `internal/tui/model.go` (add delete confirmation)
- Create: `internal/tui/confirm.go` (simple y/n overlay)

**Contracts:**
```go
// Delete confirmation overlay
type ConfirmModel struct {
    message string
    onYes   func() tea.Cmd
}
```

**Features:**
- `d` on a session shows "Delete session <title>? [y/n]"
- `y` deletes the JSONL file and its subagents/ dir, refreshes list
- `n` / `esc` cancels
- Auto-cleanup on startup: delete JSONL < 25KB, remove empty dirs (same as cc-sessions)

**Verification:**
Run: Test delete flow manually
Expected: Confirmation appears, y deletes, n cancels, list refreshes

**Commit after passing.**

[Mode: Direct]

---

## Execution
**Skill:** superpowers:subagent-driven-development
- Mode A tasks (Direct): Tasks 1, 2, 4, 5, 8
- Mode B tasks (Delegated): Tasks 3, 6, 7
