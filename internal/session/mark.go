package session

import (
	"sort"
	"time"

	"ccs/internal/types"
)

const (
	// maxCreationGap is the max time between a process start and a session's
	// first JSONL timestamp for them to be considered a match.
	maxCreationGap = 2 * time.Minute

	// recentActivityWindow is how recently a session must have been modified
	// to qualify for phase-2 matching (catches compacted/restarted sessions).
	recentActivityWindow = 5 * time.Minute
)

// MarkActiveSessions marks sessions as active based on detected claude processes.
//
// Two-phase approach per project dir:
//  1. Greedy 1:1 match: pair each process start time with the session whose
//     creation time is closest (within maxCreationGap). Sorts both ascending
//     and matches in order.
//  2. For unmatched processes: look for sessions with very recent mtime
//     (within recentActivityWindow) to catch compacted or restarted sessions.
func MarkActiveSessions(sessions []types.Session, active types.ActiveInfo) {
	now := time.Now()

	for dir, info := range active.ProjectDirs {
		// Collect session indices for this project dir
		var dirSessions []int
		for i := range sessions {
			if sessions[i].ProjectDir == dir {
				dirSessions = append(dirSessions, i)
			}
		}

		if len(dirSessions) == 0 {
			continue
		}

		// Sort process starts ascending
		starts := make([]time.Time, len(info.ProcessStarts))
		copy(starts, info.ProcessStarts)
		sort.Slice(starts, func(a, b int) bool {
			return starts[a].Before(starts[b])
		})

		// Phase 1: greedy match by creation time proximity
		// Sort candidate sessions by CreatedAt ascending
		withCreation := make([]int, 0, len(dirSessions))
		for _, idx := range dirSessions {
			if !sessions[idx].CreatedAt.IsZero() {
				withCreation = append(withCreation, idx)
			}
		}
		sort.Slice(withCreation, func(a, b int) bool {
			return sessions[withCreation[a]].CreatedAt.Before(sessions[withCreation[b]].CreatedAt)
		})

		matched := make(map[int]bool) // session indices already matched
		processMatched := make([]bool, len(starts))

		for pi, pStart := range starts {
			bestIdx := -1
			bestGap := maxCreationGap + 1

			for _, si := range withCreation {
				if matched[si] {
					continue
				}
				created := sessions[si].CreatedAt
				// Session must be created after (or very close to) process start
				gap := created.Sub(pStart)
				if gap < -10*time.Second {
					// Session created well before process — skip
					continue
				}
				if gap > maxCreationGap {
					// Past the window — since sorted, all remaining are further
					break
				}
				absGap := gap
				if absGap < 0 {
					absGap = -absGap
				}
				if absGap < bestGap {
					bestGap = absGap
					bestIdx = si
				}
			}

			if bestIdx >= 0 {
				sessions[bestIdx].IsActive = true
				matched[bestIdx] = true
				processMatched[pi] = true
			}
		}

		// Phase 2: unmatched processes → look for recently modified sessions
		unmatchedCount := 0
		for _, m := range processMatched {
			if !m {
				unmatchedCount++
			}
		}

		if unmatchedCount > 0 {
			var recentCandidates []int
			for _, idx := range dirSessions {
				if matched[idx] {
					continue
				}
				if now.Sub(sessions[idx].LastActive) <= recentActivityWindow {
					recentCandidates = append(recentCandidates, idx)
				}
			}

			// Sort by LastActive descending
			sort.Slice(recentCandidates, func(a, b int) bool {
				return sessions[recentCandidates[a]].LastActive.After(sessions[recentCandidates[b]].LastActive)
			})

			n := unmatchedCount
			if n > len(recentCandidates) {
				n = len(recentCandidates)
			}
			for _, idx := range recentCandidates[:n] {
				sessions[idx].IsActive = true
			}
		}
	}
}
