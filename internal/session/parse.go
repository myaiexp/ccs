package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"ccs/internal/types"
)

// jsonLine is the minimal structure we parse from each JSONL line.
type jsonLine struct {
	Type      string      `json:"type"`
	SessionID string      `json:"sessionId"`
	Timestamp string      `json:"timestamp"`
	IsMeta    bool        `json:"isMeta"`
	Subtype   string      `json:"subtype"`
	Content   string      `json:"content"` // top-level content for system messages
	Message   jsonMessage `json:"message"`
}

type jsonMessage struct {
	Content json.RawMessage `json:"content"`
	Usage   *jsonUsage      `json:"usage,omitempty"`
}

type jsonUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

var (
	renamedRe   = regexp.MustCompile(`Session renamed to:\s*(.+?)(?:<|$)`)
	htmlTagRe   = regexp.MustCompile(`<[^>]*>`)
	mdHeadingRe = regexp.MustCompile(`^#+\s*`)
	mdBoldRe    = regexp.MustCompile(`\*\*([^*]*)\*\*`)
)

const maxContextTokens = 200000

// ParseSession reads a JSONL file and extracts session metadata.
// Does NOT read full content — only enough for listing.
func ParseSession(fpath string) (*types.Session, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// Session ID is the filename (minus .jsonl extension). This is the canonical
	// local session ID that claude --resume expects. We never extract it from JSONL
	// content because teleported sessions contain the original web session ID in
	// their early lines, which differs from the local filename-based ID.
	sessionID := strings.TrimSuffix(filepath.Base(fpath), ".jsonl")

	var (
		sessionName  string // explicit name from /session-name
		fallbackText string
		msgCount     int
		lastUsage    *jsonUsage
		createdAt    time.Time
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 512*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry jsonLine
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if createdAt.IsZero() && entry.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
				createdAt = t
			}
		}

		switch entry.Type {
		case "user":
			msgCount++
			// Check for rename in content (must check every line, not just when title is empty)
			contentStr := extractContentString(&entry)
			if m := renamedRe.FindStringSubmatch(contentStr); m != nil {
				sessionName = strings.TrimSpace(m[1])
			}
			// Track fallback: first non-meta user message with string content
			if fallbackText == "" && !entry.IsMeta {
				if s := getStringContent(&entry); s != "" {
					fallbackText = s
				}
			}

		case "system":
			// System messages store content at top level, not under message.content
			contentStr := entry.Content
			if contentStr == "" {
				contentStr = extractContentString(&entry)
			}
			if m := renamedRe.FindStringSubmatch(contentStr); m != nil {
				sessionName = strings.TrimSpace(m[1])
			}

		case "assistant":
			if entry.Message.Usage != nil {
				lastUsage = entry.Message.Usage
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning %s: %w", fpath, err)
	}

	// Resolve title (always from first user message)
	title := "(untitled)"
	if fallbackText != "" {
		title = cleanTitle(fallbackText)
	}

	// Clean session name if present
	if sessionName != "" {
		sessionName = cleanTitle(sessionName)
	}

	// Context %
	contextPct := 0
	if lastUsage != nil {
		tokens := lastUsage.InputTokens + lastUsage.CacheCreationInputTokens + lastUsage.CacheReadInputTokens
		contextPct = tokens * 100 / maxContextTokens
	}

	shortID := sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	return &types.Session{
		ID:          sessionID,
		ShortID:     shortID,
		SessionName: sessionName,
		Title:       title,
		FilePath:    fpath,
		FileSize:    info.Size(),
		ContextPct:  contextPct,
		MsgCount:    msgCount,
		CreatedAt:   createdAt,
		LastActive:  info.ModTime(),
	}, nil
}

// extractContentString gets a string representation of message content,
// handling both string and array forms.
func extractContentString(entry *jsonLine) string {
	if entry.Message.Content == nil {
		return ""
	}
	// Try string first
	var s string
	if err := json.Unmarshal(entry.Message.Content, &s); err == nil {
		return s
	}
	// Try array of objects with "text" fields
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(entry.Message.Content, &parts); err == nil {
		var sb strings.Builder
		for _, p := range parts {
			if p.Text != "" {
				sb.WriteString(p.Text)
				sb.WriteByte(' ')
			}
		}
		return sb.String()
	}
	return ""
}

// getStringContent returns content only if it's a plain string (not array).
// Filters out command-like messages (content starting with '<').
func getStringContent(entry *jsonLine) string {
	if entry.Message.Content == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(entry.Message.Content, &s); err == nil {
		// Skip command-like content
		if strings.HasPrefix(s, "<") {
			return ""
		}
		return s
	}
	return ""
}

func stripHTML(s string) string {
	return strings.TrimSpace(htmlTagRe.ReplaceAllString(s, ""))
}

// cleanTitle extracts a clean, single-line title from raw message content.
func cleanTitle(s string) string {
	s = stripHTML(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = mdHeadingRe.ReplaceAllString(s, "")
	s = mdBoldRe.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.TrimLeft(s, "->+ ")
	return strings.TrimSpace(s)
}

// sessionFile holds info needed to parse a session file.
type sessionFile struct {
	path     string
	projName string
	projPath string
	modTime  time.Time
	size     int64
}

// DiscoverSessions finds all session JSONL files in the given projects dir,
// parses them, and returns sorted by LastActive descending.
// Skips files in subagents/ dirs and files < 25KB.
// Uses a file metadata cache to skip re-parsing unchanged files,
// and parses uncached files in parallel.
func DiscoverSessions(projectsDir string) ([]types.Session, error) {
	projDirs, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, fmt.Errorf("reading projects dir: %w", err)
	}

	cache := loadCache()

	var cached []types.Session
	var toParse []sessionFile
	validPaths := make(map[string]bool)

	for _, pd := range projDirs {
		if !pd.IsDir() {
			continue
		}

		projName, projPath := DecodeProjectDir(pd.Name())
		dirPath := filepath.Join(projectsDir, pd.Name())

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}

			fpath := filepath.Join(dirPath, entry.Name())

			if strings.Contains(fpath, "/subagents/") {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			if info.Size() < 25*1024 {
				continue
			}

			validPaths[fpath] = true

			// Check cache
			if sess, ok := cache.get(fpath, info.ModTime(), info.Size()); ok {
				sess.ProjectName = projName
				sess.ProjectDir = projPath
				cached = append(cached, *sess)
				continue
			}

			toParse = append(toParse, sessionFile{
				path:     fpath,
				projName: projName,
				projPath: projPath,
				modTime:  info.ModTime(),
				size:     info.Size(),
			})
		}
	}

	// Parse uncached files in parallel
	type result struct {
		sess types.Session
		sf   sessionFile
	}

	workers := 8
	if len(toParse) < workers {
		workers = len(toParse)
	}

	jobs := make(chan sessionFile, len(toParse))
	results := make(chan result, len(toParse))

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sf := range jobs {
				sess, err := ParseSession(sf.path)
				if err != nil {
					continue
				}
				sess.ProjectName = sf.projName
				sess.ProjectDir = sf.projPath
				results <- result{sess: *sess, sf: sf}
			}
		}()
	}

	for _, sf := range toParse {
		jobs <- sf
	}
	close(jobs)

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	parsed := make([]types.Session, 0, len(toParse))
	for r := range results {
		cache.set(r.sf.path, r.sf.modTime, r.sf.size, &r.sess)
		parsed = append(parsed, r.sess)
	}

	// Save cache (prunes deleted files)
	cache.save(validPaths)

	// Merge cached + parsed
	sessions := make([]types.Session, 0, len(cached)+len(parsed))
	sessions = append(sessions, cached...)
	sessions = append(sessions, parsed...)

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActive.After(sessions[j].LastActive)
	})

	return sessions, nil
}

// LoadSessions discovers sessions, refreshes tracker state, matches new sessions,
// and marks active sessions. This is the canonical session loading sequence used
// by both startup (main.go) and refresh (handleRefresh).
func LoadSessions(projectsDir string, tracker *Tracker) ([]types.Session, error) {
	sessions, err := DiscoverSessions(projectsDir)
	if err != nil {
		return nil, err
	}
	tracker.Refresh()
	tracker.MatchNewSession(sessions)
	tracker.MarkActive(sessions)
	return sessions, nil
}

