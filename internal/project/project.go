package project

import (
	"ccs/internal/types"
	"sort"
)

// DiscoverProjects extracts unique projects from session data,
// merges with active detection and config for hidden status.
func DiscoverProjects(sessions []types.Session, activeDirs map[string]bool, cfg *types.Config) []types.Project {
	hiddenSet := make(map[string]bool, len(cfg.HiddenProjects))
	for _, h := range cfg.HiddenProjects {
		hiddenSet[h] = true
	}

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
				Hidden:     hiddenSet[s.ProjectName],
			}
		}
	}

	// Also mark active from activeDirs for projects that might not have recent sessions
	for encodedDir := range activeDirs {
		for _, p := range byName {
			if p.HasActive {
				continue
			}
			// Check if any session in this project matches
			for _, s := range sessions {
				if s.ProjectName == p.Name && activeDirs[encodedDir] {
					p.HasActive = true
					break
				}
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
