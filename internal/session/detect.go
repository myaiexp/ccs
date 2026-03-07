package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ccs/internal/types"
)

// DetectActive returns info about running claude processes:
// which project dirs have active sessions and which specific session IDs
// are being resumed.
func DetectActive() types.ActiveInfo {
	info := types.ActiveInfo{
		ProjectDirs: make(map[string]bool),
		SessionIDs:  make(map[string]bool),
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return info
	}

	selfPID := os.Getpid()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		if pid == selfPID {
			continue
		}

		cmdlineBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}

		// cmdline is null-delimited; split to get argv
		args := strings.Split(string(cmdlineBytes), "\x00")
		if len(args) == 0 || args[0] == "" {
			continue
		}

		// Check if argv[0] is a claude binary (path ends with "/claude" or is just "claude")
		bin := args[0]
		baseName := filepath.Base(bin)
		if baseName != "claude" {
			continue
		}

		// Read the working directory of this process
		cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if err != nil {
			continue
		}

		encoded := encodePathToProjectDir(cwd)
		info.ProjectDirs[encoded] = true

		// Look for --resume <session-id> in args
		for i, arg := range args {
			if arg == "--resume" && i+1 < len(args) && args[i+1] != "" {
				info.SessionIDs[args[i+1]] = true
				break
			}
		}
	}

	return info
}

// encodePathToProjectDir converts a filesystem path to the encoded directory
// name format used by Claude's session storage.
// "/home/mse/Projects/foo" → "-home-mse-Projects-foo"
func encodePathToProjectDir(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}
