package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// Launch sends a launch request to the running ccs instance.
func Launch(socketPath string, req LaunchRequest) (LaunchResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return LaunchResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	msg := Message{
		Type: "launch",
		Data: data,
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return LaunchResponse{}, fmt.Errorf("connect to ccs: %w", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return LaunchResponse{}, fmt.Errorf("send request: %w", err)
	}

	var resp LaunchResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return LaunchResponse{}, fmt.Errorf("read response: %w", err)
	}

	return resp, nil
}

// NotifyExit sends an exit notification (fire-and-forget).
func NotifyExit(socketPath string, notif ExitNotification) error {
	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	msg := Message{
		Type: "exit",
		Data: data,
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect to ccs: %w", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return fmt.Errorf("send notification: %w", err)
	}

	return nil
}

// SocketPath returns the default socket path (~/.cache/ccs/ccs.sock), expanding ~.
func SocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/ccs.sock"
	}
	return filepath.Join(home, ".cache", "ccs", "ccs.sock")
}
