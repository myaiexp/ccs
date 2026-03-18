package project

import (
	"ccs/internal/types"
	"os"
	"path/filepath"
	"sort"
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

// DiscoverProjects extracts unique projects from session data.
// Sessions must already have IsActive set.
func DiscoverProjects(sessions []types.Session) []types.Project {
	// Collect unique projects, track most recent session per project
	byName := make(map[string]*types.Project)
	for _, s := range sessions {
		if p, ok := byName[s.ProjectName]; ok {
			if s.LastActive.After(p.LastActive) {
				p.LastActive = s.LastActive
			}
			if s.IsActive {
				p.HasActive = true
			}
		} else {
			byName[s.ProjectName] = &types.Project{
				Name:       s.ProjectName,
				Dir:        s.ProjectDir,
				LastActive: s.LastActive,
				HasActive:  s.IsActive,
			}
		}
	}

	projects := make([]types.Project, 0, len(byName))
	for _, p := range byName {
		projects = append(projects, *p)
	}

	sort.Slice(projects, func(i, j int) bool {
		// Active first
		if projects[i].HasActive != projects[j].HasActive {
			return projects[i].HasActive
		}
		// Then by recency
		return projects[i].LastActive.After(projects[j].LastActive)
	})

	return projects
}
