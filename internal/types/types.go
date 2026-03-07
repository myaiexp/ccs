package types

import "time"

type Session struct {
	ID          string
	ShortID     string
	ProjectName string
	ProjectDir  string
	Title       string
	FirstMsg    string // Full first user message (up to 500 chars, for detail pane)
	ContextPct  int
	MsgCount    int
	FileSize    int64
	LastActive  time.Time
	IsActive    bool
	FilePath    string
}

type Project struct {
	Name       string
	Dir        string
	LastActive time.Time
	HasActive  bool
	Hidden     bool
}

type Config struct {
	HiddenProjects  []string `toml:"hidden_projects"`
	HiddenSessions  []string `toml:"hidden_sessions"`
	ClaudeFlags     []string `toml:"claude_flags"`
	RelativeNumbers bool     `toml:"relative_numbers"`
}

// SortField determines which field to sort by.
type SortField int

const (
	SortByTime SortField = iota
	SortByContext
	SortBySize
	SortByName
)

func (s SortField) String() string {
	switch s {
	case SortByTime:
		return "time"
	case SortByContext:
		return "ctx%"
	case SortBySize:
		return "size"
	case SortByName:
		return "name"
	}
	return ""
}

func (s SortField) Next() SortField {
	return (s + 1) % 4
}

// SortDir is ascending or descending.
type SortDir int

const (
	SortDesc SortDir = iota
	SortAsc
)

func (d SortDir) Toggle() SortDir {
	if d == SortDesc {
		return SortAsc
	}
	return SortDesc
}

func (d SortDir) String() string {
	if d == SortAsc {
		return "↑"
	}
	return "↓"
}

// ActiveInfo holds the results of active session detection.
type ActiveInfo struct {
	ProjectDirs map[string]bool   // encoded project dirs with an active claude
	SessionIDs  map[string]bool   // specific session IDs (from --resume flag)
}
