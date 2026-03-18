package naming

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Result holds the outcome of an auto-naming attempt.
type Result struct {
	SessionID string
	Name      string // empty on SKIP/error
	Err       error
}

const promptTemplate = `Name this Claude Code session based on the terminal output below.
Reply with ONLY a 3-6 word task description (e.g. "config-sync autopull setup").
Focus on what task the session is accomplishing, not specific tools or code details.
If there isn't enough context, reply with exactly SKIP.

Terminal output:
%s`

// GenerateName shells out to claude --print --model haiku for auto-naming.
func GenerateName(sessionID, contextText string, maxLines int) Result {
	lines := strings.Split(contextText, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	content := strings.Join(lines, "\n")
	if strings.TrimSpace(content) == "" {
		return Result{SessionID: sessionID}
	}

	prompt := fmt.Sprintf(promptTemplate, content)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "--print", "--model", "haiku", "--no-session-persistence")
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stderr = io.Discard

	out, err := cmd.Output()
	if err != nil {
		return Result{SessionID: sessionID, Err: fmt.Errorf("claude: %w", err)}
	}

	name := parseResponse(string(out))
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

// TailFileLines reads the last maxLines lines from a file as plain text.
func TailFileLines(path string, maxLines int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Read last 32KB
	const tailSize = 32 * 1024
	stat, err := f.Stat()
	if err != nil {
		return ""
	}

	offset := stat.Size() - tailSize
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}
