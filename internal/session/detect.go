package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ccs/internal/types"
)

// DetectActive scans /proc for running claude processes and returns
// per-project-dir process counts and earliest start times.
func DetectActive() types.ActiveInfo {
	info := types.ActiveInfo{
		ProjectDirs: make(map[string]types.ProjectActiveInfo),
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

		// Check if argv[0] is a claude binary
		baseName := filepath.Base(args[0])
		if baseName != "claude" {
			continue
		}

		// Read the working directory of this process
		cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if err != nil {
			continue
		}

		// Get process start time from /proc/<pid> directory stat
		procDir := filepath.Join("/proc", entry.Name())
		procStat, err := os.Stat(procDir)
		if err != nil {
			continue
		}
		startTime := procStat.ModTime()

		// Accumulate per-project-dir
		existing := info.ProjectDirs[cwd]
		existing.ProcessStarts = append(existing.ProcessStarts, startTime)
		info.ProjectDirs[cwd] = existing
	}

	return info
}
