package types

import "time"

// ActiveSource indicates how an active session was detected.
type ActiveSource int

const (
	SourceInactive ActiveSource = iota
	SourceProc                  // found via /proc, no tmux window
	SourceTmux                  // launched from ccs, has tmux window
)

// StateStatus represents the merged lifecycle state of a session.
type StateStatus int

const (
	StatusUntracked StateStatus = iota
	StatusDone
	StatusOpen
	StatusActive
)

func (s StateStatus) String() string {
	switch s {
	case StatusUntracked:
		return "untracked"
	case StatusDone:
		return "done"
	case StatusOpen:
		return "open"
	case StatusActive:
		return "active"
	}
	return ""
}

type Session struct {
	ID           string
	ShortID      string
	ProjectName  string
	ProjectDir   string
	SessionName  string // Explicit name from /session-name (empty if not renamed)
	Title        string
	ContextPct   int
	MsgCount     int
	FileSize     int64
	CreatedAt    time.Time
	LastActive   time.Time
	IsActive     bool
	ActiveSource ActiveSource
	StateStatus  StateStatus
	FilePath     string
}

type Project struct {
	Name       string
	Dir        string
	LastActive time.Time
	HasActive  bool
}

type Config struct {
	HiddenSessions  []string `toml:"hidden_sessions"`
	ClaudeFlags     []string `toml:"claude_flags"`
	RelativeNumbers bool     `toml:"relative_numbers"`
	TmuxSessionName string   `toml:"tmux_session_name"`
	ActivityLines   int      `toml:"activity_lines"`
	AutoNameLines   int      `toml:"auto_name_lines"`
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

const sortFieldCount = SortByName + 1

func (s SortField) Next() SortField {
	return (s + 1) % sortFieldCount
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
