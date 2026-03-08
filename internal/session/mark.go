package session

import (
	"sort"

	"ccs/internal/types"
)

// MarkActiveSessions marks sessions as active based on detected claude processes.
// For each project dir with N running processes, the N most recently modified
// sessions (with LastActive >= earliest process start time) are marked active.
func MarkActiveSessions(sessions []types.Session, active types.ActiveInfo) {
	for dir, info := range active.ProjectDirs {
		// Collect indices of sessions in this project dir with recent enough mtime
		var candidates []int
		for i := range sessions {
			if sessions[i].ProjectDir != dir {
				continue
			}
			if sessions[i].LastActive.Before(info.EarliestStart) {
				continue
			}
			candidates = append(candidates, i)
		}

		// Sort candidates by LastActive descending
		sort.Slice(candidates, func(a, b int) bool {
			return sessions[candidates[a]].LastActive.After(sessions[candidates[b]].LastActive)
		})

		// Mark top N as active
		n := info.Count
		if n > len(candidates) {
			n = len(candidates)
		}
		for _, idx := range candidates[:n] {
			sessions[idx].IsActive = true
		}
	}
}
