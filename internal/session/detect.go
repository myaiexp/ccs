package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DetectActive returns a map of encoded project directory names that have
// a running claude process. E.g. "-home-mse-Projects-foo" → true
func DetectActive() map[string]bool {
	active := make(map[string]bool)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return active
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
		active[encoded] = true
	}

	return active
}

// encodePathToProjectDir converts a filesystem path to the encoded directory
// name format used by Claude's session storage.
// "/home/mse/Projects/foo" → "-home-mse-Projects-foo"
func encodePathToProjectDir(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}
