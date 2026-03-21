package ipc

import "encoding/json"

// Message wraps all IPC messages with a type discriminator.
type Message struct {
	Type string          `json:"type"` // "launch", "exit"
	Data json.RawMessage `json:"data"`
}

// LaunchRequest asks the running ccs instance to open a new session.
type LaunchRequest struct {
	ProjectDir string `json:"project_dir"`
	ResumeID   string `json:"resume_id,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	OnDone     string `json:"on_done,omitempty"`
}

// LaunchResponse is returned after processing a LaunchRequest.
type LaunchResponse struct {
	OK        bool   `json:"ok"`
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ExitNotification is sent by the pane-exited hook to signal session completion.
type ExitNotification struct {
	WindowID string `json:"window_id"`
}
