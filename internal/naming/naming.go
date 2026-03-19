package naming

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Result holds the outcome of an auto-naming attempt.
type Result struct {
	SessionID string
	Name      string // empty on SKIP/error
	Err       error
}

// SummaryResult holds the outcome of a summary generation.
type SummaryResult struct {
	SessionID string
	Summary   string
	Err       error
}

const statusPrompt = `Describe what this Claude Code session is currently doing in ONE line (max 15 words).
Focus on the current task and progress, not specific tools or file names.
If there isn't enough context, reply with exactly SKIP.

Terminal output:
%s`

const condensePrompt = `Based on these periodic status updates from a Claude Code session, write a short name (3-8 words) that describes what the session accomplished overall.
Reply with ONLY the name, nothing else.

Status updates (oldest to newest):
%s`

const comprehensivePrompt = `Based on these periodic status updates from a Claude Code session, write a 2-3 sentence summary of what was accomplished.
Start with a capital letter. Be specific and concise. Max 200 characters total.

Status updates (oldest to newest):
%s`

// --- Logging ---

var (
	logMu   sync.Mutex
	logFile *os.File
)

func logPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "ccs", "naming.log")
}

// LogEntry writes a timestamped line to the naming log.
func LogEntry(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()

	if logFile == nil {
		path := logPath()
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
		logFile = f
	}

	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(logFile, "[%s] %s\n", ts, msg)
}

// LogPath returns the path to the naming log file.
func LogPath() string {
	return logPath()
}

// --- Core ---

// callHaiku sends a prompt to claude --print --model haiku and returns the response.
func callHaiku(operation, sessionID, prompt string, timeout time.Duration) (string, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "--print", "--model", "haiku", "--no-session-persistence")
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stderr = io.Discard

	out, err := cmd.Output()
	elapsed := time.Since(start)

	if err != nil {
		LogEntry("ERROR %s session=%s elapsed=%s err=%v", operation, sessionID[:8], elapsed, err)
		return "", fmt.Errorf("claude: %w", err)
	}

	result := strings.TrimSpace(string(out))
	LogEntry("%s session=%s %.1fs → %q", operation, sessionID[:8], elapsed.Seconds(), truncateLog(result, 200))
	return result, nil
}

func truncateLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("... (%d bytes truncated)", len(s)-max)
}

// GenerateStatus produces a one-line status summary from pane capture content.
func GenerateStatus(sessionID, paneContent string, maxLines int) Result {
	lines := strings.Split(paneContent, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	content := strings.Join(lines, "\n")
	if strings.TrimSpace(content) == "" {
		return Result{SessionID: sessionID}
	}

	prompt := fmt.Sprintf(statusPrompt, content)
	raw, err := callHaiku("status", sessionID, prompt, 60*time.Second)
	if err != nil {
		return Result{SessionID: sessionID, Err: err}
	}

	name := parseResponse(raw)
	if name == "" {
	}
	return Result{SessionID: sessionID, Name: name}
}

// CondenseName produces a short name from a list of status update texts.
func CondenseName(sessionID string, statusTexts []string) Result {
	if len(statusTexts) == 0 {
		return Result{SessionID: sessionID}
	}

	numbered := make([]string, len(statusTexts))
	for i, t := range statusTexts {
		numbered[i] = fmt.Sprintf("%d. %s", i+1, t)
	}
	prompt := fmt.Sprintf(condensePrompt, strings.Join(numbered, "\n"))

	raw, err := callHaiku("condense", sessionID, prompt, 60*time.Second)
	if err != nil {
		return Result{SessionID: sessionID, Err: err}
	}

	name := parseResponse(raw)
	return Result{SessionID: sessionID, Name: name}
}

// GenerateComprehensiveSummary produces a multi-line summary from all status updates.
func GenerateComprehensiveSummary(sessionID string, statusTexts []string) SummaryResult {
	if len(statusTexts) == 0 {
		return SummaryResult{SessionID: sessionID}
	}

	numbered := make([]string, len(statusTexts))
	for i, t := range statusTexts {
		numbered[i] = fmt.Sprintf("%d. %s", i+1, t)
	}
	prompt := fmt.Sprintf(comprehensivePrompt, strings.Join(numbered, "\n"))

	raw, err := callHaiku("comprehensive", sessionID, prompt, 60*time.Second)
	if err != nil {
		return SummaryResult{SessionID: sessionID, Err: err}
	}

	summary := strings.TrimSpace(raw)
	if summary == "" || strings.EqualFold(summary, "SKIP") {
		return SummaryResult{SessionID: sessionID}
	}
	return SummaryResult{SessionID: sessionID, Summary: summary}
}

// GenerateName shells out to claude --print --model haiku for auto-naming.
// Kept for backwards compatibility with manual N key trigger.
func GenerateName(sessionID, contextText string, maxLines int) Result {
	lines := strings.Split(contextText, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	content := strings.Join(lines, "\n")
	if strings.TrimSpace(content) == "" {
		return Result{SessionID: sessionID}
	}

	prompt := fmt.Sprintf(statusPrompt, content)
	raw, err := callHaiku("name", sessionID, prompt, 60*time.Second)
	if err != nil {
		return Result{SessionID: sessionID, Err: err}
	}

	name := parseResponse(raw)
	return Result{SessionID: sessionID, Name: name}
}

// parseResponse extracts a session name from claude's raw output.
// Returns empty string for "SKIP" or empty responses.
func parseResponse(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "SKIP") {
		return ""
	}
	// Take first line only
	if idx := strings.IndexByte(raw, '\n'); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(raw)
}
