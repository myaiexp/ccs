package session

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DecodeProjectDir converts an encoded directory name to a display name and absolute path.
// The encoding replaces "/" with "-", making it ambiguous for names containing dashes.
// Claude Code also encodes "." as "-", so "mase.fi" becomes "mase-fi".
// We use known prefix patterns to disambiguate (same approach as cc-sessions),
// then verify the path exists and try dot substitutions if it doesn't.
//
// "-home-mse-Projects-poe-proof" → ("poe-proof", "/home/mse/Projects/poe-proof")
// "-home-mse" → ("~", "/home/mse")
// "-home-mse--openclaw" → (".openclaw", "/home/mse/.openclaw")
// "-home-mse-Projects-mase-fi" → ("mase.fi", "/home/mse/Projects/mase.fi")
func DecodeProjectDir(encoded string) (name string, absPath string) {
	if encoded == "" {
		return "", ""
	}

	// Try both Linux (/home/) and macOS (/Users/) prefixes
	type homePrefix struct {
		dir string // "/home/" or "/Users/"
		re  struct {
			projects, dotDir, homeDir, homeSubdir *regexp.Regexp
		}
	}
	prefixes := []homePrefix{
		{dir: "/home/", re: struct {
			projects, dotDir, homeDir, homeSubdir *regexp.Regexp
		}{projectsRe, dotDirRe, homeDirRe, homeSubdirRe}},
		{dir: "/Users/", re: struct {
			projects, dotDir, homeDir, homeSubdir *regexp.Regexp
		}{macProjectsRe, macDotDirRe, macHomeDirRe, macHomeSubdirRe}},
	}

	for _, pfx := range prefixes {
		// Pattern: -{prefix}-{user}-Projects-{rest}
		if m := pfx.re.projects.FindStringSubmatch(encoded); m != nil {
			user := m[1]
			rest := m[2]
			parentDir := pfx.dir + user + "/Projects/"
			absPath = parentDir + rest
			name = rest
			// If the decoded path doesn't exist, try dot substitutions in the project name
			if resolved, ok := resolveWithDots(parentDir, rest); ok {
				absPath = resolved
				name = filepath.Base(resolved)
			}
			return
		}

		// Pattern: -{prefix}-{user}--{rest} → dot-prefixed dir in home
		if m := pfx.re.dotDir.FindStringSubmatch(encoded); m != nil {
			user := m[1]
			rest := m[2]
			absPath = pfx.dir + user + "/." + rest
			name = "." + rest
			return
		}

		// Pattern: -{prefix}-{user} exactly → home dir
		if m := pfx.re.homeDir.FindStringSubmatch(encoded); m != nil {
			user := m[1]
			absPath = pfx.dir + user
			name = "~"
			return
		}

		// Pattern: -{prefix}-{user}-{rest} → subdir of home
		if m := pfx.re.homeSubdir.FindStringSubmatch(encoded); m != nil {
			user := m[1]
			rest := m[2]
			absPath = pfx.dir + user + "/" + rest
			name = rest
			return
		}
	}

	// Fallback: replace leading dash, split on remaining dashes as path components
	absPath = "/" + strings.ReplaceAll(encoded[1:], "-", "/")
	parts := strings.Split(encoded[1:], "-")
	name = parts[len(parts)-1]
	return
}

// resolveWithDots handles the ambiguity where Claude Code encodes "." as "-".
// If the direct path exists, returns false (no resolution needed).
// Otherwise, tries replacing each dash in the name with a dot to find a match.
// For example: "mase-fi" → tries "mase.fi" which exists → returns the resolved path.
func resolveWithDots(parentDir, name string) (string, bool) {
	direct := parentDir + name
	if _, err := os.Stat(direct); err == nil {
		return "", false // direct path exists, no resolution needed
	}

	// Find positions of dashes in the name
	dashPositions := []int{}
	for i, c := range name {
		if c == '-' {
			dashPositions = append(dashPositions, i)
		}
	}
	if len(dashPositions) == 0 {
		return "", false
	}

	// Try each single-dash-to-dot substitution (most common case: one dot in name)
	for _, pos := range dashPositions {
		candidate := name[:pos] + "." + name[pos+1:]
		path := parentDir + candidate
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}

	return "", false
}

var (
	// Linux: -home-{user}-...
	projectsRe   = regexp.MustCompile(`^-home-([^-]+)-Projects-(.+)$`)
	dotDirRe     = regexp.MustCompile(`^-home-([^-]+)--(.+)$`)
	homeDirRe    = regexp.MustCompile(`^-home-([^-]+)$`)
	homeSubdirRe = regexp.MustCompile(`^-home-([^-]+)-(.+)$`)

	// macOS: -Users-{user}-...
	macProjectsRe   = regexp.MustCompile(`^-Users-([^-]+)-Projects-(.+)$`)
	macDotDirRe     = regexp.MustCompile(`^-Users-([^-]+)--(.+)$`)
	macHomeDirRe    = regexp.MustCompile(`^-Users-([^-]+)$`)
	macHomeSubdirRe = regexp.MustCompile(`^-Users-([^-]+)-(.+)$`)
)
