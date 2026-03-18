package project

import (
	"os"
	"path/filepath"
)

// ProjectDir represents a project directory on disk (~/Projects/*).
type ProjectDir struct {
	Name string
	Path string
}

// ScanProjectDirs scans the given root directory for project directories.
func ScanProjectDirs(root string) []ProjectDir {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []ProjectDir
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		dirs = append(dirs, ProjectDir{
			Name: name,
			Path: filepath.Join(root, name),
		})
	}
	return dirs
}
