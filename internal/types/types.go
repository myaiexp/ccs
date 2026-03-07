package types

import "time"

type Session struct {
	ID          string
	ShortID     string
	ProjectName string
	ProjectDir  string
	Title       string
	ContextPct  int
	MsgCount    int
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
	HiddenProjects []string `toml:"hidden_projects"`
	ClaudeFlags    []string `toml:"claude_flags"`
}
