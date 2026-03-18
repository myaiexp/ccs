package capture

import (
	"fmt"
	"strings"
)

// TransformPaneContent applies Claude Code-specific content transformations:
// strips status bar, removes trailing noise, and collapses task lists.
func TransformPaneContent(content string) string {
	content = stripStatusBar(content)
	content = stripTrailingNoise(content)
	content = collapseTaskList(content)
	return content
}

// stripStatusBar removes status bar / HUD lines from the bottom of captured pane content.
// Scans the last 15 lines from the bottom for the topmost box-drawing separator line
// and strips everything from there downward (HUD content, prompt lines, etc).
func stripStatusBar(content string) string {
	lines := strings.Split(content, "\n")
	cutIdx := -1
	searchStart := len(lines) - 15
	if searchStart < 0 {
		searchStart = 0
	}
	for i := searchStart; i < len(lines); i++ {
		if isBoxDrawingLine(lines[i]) {
			if cutIdx == -1 {
				cutIdx = i
			}
		}
	}
	if cutIdx > 0 {
		lines = lines[:cutIdx]
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// stripTrailingNoise removes trailing empty lines and spinner/status lines
// from the bottom of captured content.
func stripTrailingNoise(content string) string {
	lines := strings.Split(content, "\n")
	for len(lines) > 0 {
		line := strings.TrimSpace(lines[len(lines)-1])
		if line == "" || IsSpinnerLine(line) {
			lines = lines[:len(lines)-1]
		} else {
			break
		}
	}
	return strings.Join(lines, "\n")
}

// IsSpinnerLine detects Claude's activity spinner lines (✻ Thinking..., etc).
func IsSpinnerLine(line string) bool {
	for _, r := range line {
		if r == ' ' {
			continue
		}
		return r == '*' || r == '✻' || r == '✱' || r == '✳' || r == '·' ||
			r == '•' || r == '∗' ||
			(r >= 0x2800 && r <= 0x28FF)
	}
	return false
}

// isBoxDrawingLine returns true if a line is predominantly box-drawing characters.
func isBoxDrawingLine(line string) bool {
	if len(line) == 0 {
		return false
	}
	boxCount := 0
	total := 0
	for _, r := range line {
		if r == ' ' || r == '\t' {
			continue
		}
		total++
		if r >= 0x2500 && r <= 0x257F {
			boxCount++
		}
	}
	return total > 10 && boxCount*100/total > 80
}

// collapseTaskList detects Claude Code task lists and collapses them to show
// only the currently active task.
func collapseTaskList(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	var activeTasks []string
	totalTasks := 0
	completedTasks := 0
	inTaskBlock := false

	emitTaskBlock := func() {
		if totalTasks > 0 && len(activeTasks) > 0 {
			if completedTasks > 0 {
				result = append(result, fmt.Sprintf("  %d/%d done", completedTasks, totalTasks))
			}
			result = append(result, activeTasks...)
		}
		activeTasks = nil
		totalTasks = 0
		completedTasks = 0
		inTaskBlock = false
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		marker, isTodo := TodoLineMarker(trimmed)
		if isTodo {
			inTaskBlock = true
			totalTasks++
			switch marker {
			case "done":
				completedTasks++
			case "active":
				activeTasks = append(activeTasks, line)
			case "pending":
			}
		} else {
			if inTaskBlock {
				emitTaskBlock()
			}
			result = append(result, line)
		}
	}
	if inTaskBlock {
		emitTaskBlock()
	}

	return strings.Join(result, "\n")
}

// TodoLineMarker checks if a line looks like a Claude Code task/todo line.
// Returns the marker type ("done", "active", "pending") and true, or ("", false).
func TodoLineMarker(line string) (string, bool) {
	for _, r := range line {
		if r == ' ' || r == '\t' {
			continue
		}
		switch {
		case r == '✓' || r == '✔' || r == '\u2705':
			return "done", true
		case r == '■' || r == '▪' || r == '▸':
			return "active", true
		case r == '□' || r == '▫' || r == '○':
			return "pending", true
		default:
			return "", false
		}
	}
	return "", false
}
