package activity

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry type constants.
const (
	EntryToolUse = "tool_use"
	EntryText    = "text"
)

// Entry represents a single activity extracted from a JSONL assistant message.
type Entry struct {
	Type      string    // EntryToolUse or EntryText
	Tool      string    // "Read", "Edit", "Bash", etc. Empty for text.
	Summary   string    // "Edit model.go", "Bash: go test ./...", "Fixed the import..."
	Timestamp time.Time
}

// jsonLine is the top-level structure of a JSONL line.
type jsonLine struct {
	Type      string      `json:"type"`
	Message   jsonMessage `json:"message"`
	Timestamp string      `json:"timestamp"`
}

type jsonMessage struct {
	Role    string           `json:"role"`
	Content []jsonContentBlock `json:"content"`
}

type jsonContentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name,omitempty"`
	Text  string          `json:"text,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type toolInput struct {
	FilePath string `json:"file_path"`
	Command  string `json:"command"`
	Pattern  string `json:"pattern"`
}

// ExtractFromLine parses a single JSONL line and returns activity entries.
// Returns nil if the line is not an assistant message or is malformed.
func ExtractFromLine(line []byte) []Entry {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil
	}

	var l jsonLine
	if err := json.Unmarshal(line, &l); err != nil {
		return nil
	}

	if l.Type != "assistant" {
		return nil
	}

	var ts time.Time
	if l.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339, l.Timestamp); err == nil {
			ts = parsed
		}
	}

	var entries []Entry
	for _, block := range l.Message.Content {
		switch block.Type {
		case EntryToolUse:
			e := Entry{
				Type:      EntryToolUse,
				Tool:      block.Name,
				Summary:   buildToolSummary(block.Name, block.Input),
				Timestamp: ts,
			}
			entries = append(entries, e)
		case EntryText:
			if block.Text == "" {
				continue
			}
			summary := firstLine(block.Text, 200)
			e := Entry{
				Type:      EntryText,
				Summary:   summary,
				Timestamp: ts,
			}
			entries = append(entries, e)
		}
	}

	return entries
}

// TailFile reads the last ~32KB of a JSONL file, extracts activity entries,
// and returns the latest maxEntries (newest last in file = newest first in result).
func TailFile(path string, maxEntries int) []Entry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	const tailSize = 32 * 1024

	info, err := f.Stat()
	if err != nil {
		return nil
	}

	size := info.Size()
	offset := int64(0)
	if size > tailSize {
		offset = size - tailSize
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil
		}
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")

	// Discard first partial line if we seeked into the middle of the file.
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:]
	}

	var all []Entry
	for _, line := range lines {
		entries := ExtractFromLine([]byte(line))
		all = append(all, entries...)
	}

	// Return newest first: reverse the slice, then cap at maxEntries.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}

	if len(all) > maxEntries {
		all = all[:maxEntries]
	}

	return all
}

// FormatEntry returns a human-readable display string for an entry.
func FormatEntry(e Entry) string {
	if e.Type != EntryToolUse {
		return e.Summary
	}
	switch e.Tool {
	case "Read":
		return "Reading " + extractFilename(e.Summary)
	case "Edit":
		return "Editing " + extractFilename(e.Summary)
	case "Write":
		return "Writing " + extractFilename(e.Summary)
	case "Bash":
		return "Running " + strings.TrimPrefix(e.Summary, "Bash: ")
	case "Grep":
		return "Searching " + strings.TrimPrefix(e.Summary, "Grep: ")
	case "Glob":
		return "Finding " + strings.TrimPrefix(e.Summary, "Glob: ")
	default:
		return e.Tool
	}
}

func buildToolSummary(name string, rawInput json.RawMessage) string {
	var inp toolInput
	// Best effort parse; fields will be zero-value if missing.
	_ = json.Unmarshal(rawInput, &inp)

	switch name {
	case "Read", "Edit", "Write":
		if inp.FilePath != "" {
			return name + ": " + filepath.Base(inp.FilePath)
		}
		return name
	case "Bash":
		if inp.Command != "" {
			return "Bash: " + firstLine(inp.Command, 200)
		}
		return "Bash"
	case "Grep", "Glob":
		if inp.Pattern != "" {
			return name + ": " + firstLine(inp.Pattern, 200)
		}
		return name
	default:
		return name
	}
}

// extractFilename extracts the filename part from a summary like "Edit: model.go".
func extractFilename(summary string) string {
	if idx := strings.Index(summary, ": "); idx >= 0 {
		return summary[idx+2:]
	}
	return summary
}

func firstLine(text string, maxLen int) string {
	line := text
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		line = text[:idx]
	}
	return truncate(strings.TrimSpace(line), maxLen)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// TailFileLines reads the last maxLines lines from a file as plain text.
func TailFileLines(path string, maxLines int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

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

