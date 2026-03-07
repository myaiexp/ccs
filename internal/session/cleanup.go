package session

import (
	"os"
	"path/filepath"
	"strings"
)

// Cleanup removes tiny JSONL files (<25KB) and empty directories.
func Cleanup(projectsDir string) {
	filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if strings.Contains(path, "/subagents/") {
			return nil
		}
		if info.Size() < 25*1024 {
			os.Remove(path)
		}
		return nil
	})

	// Remove empty directories
	filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || path == projectsDir {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err == nil && len(entries) == 0 {
			os.Remove(path)
		}
		return nil
	})
}
