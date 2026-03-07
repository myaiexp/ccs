package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"ccs/internal/types"
)

// cacheEntry stores parsed session metadata alongside file identity.
type cacheEntry struct {
	ModTime  time.Time     `json:"mod_time"`
	Size     int64         `json:"size"`
	Session  types.Session `json:"session"`
}

type sessionCache struct {
	Entries map[string]cacheEntry `json:"entries"` // keyed by filepath
}

func cachePath() string {
	dir, _ := os.UserCacheDir()
	return filepath.Join(dir, "ccs", "sessions.json")
}

func loadCache() *sessionCache {
	c := &sessionCache{Entries: make(map[string]cacheEntry)}
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, c)
	if c.Entries == nil {
		c.Entries = make(map[string]cacheEntry)
	}
	return c
}

func (c *sessionCache) get(fpath string, modTime time.Time, size int64) (*types.Session, bool) {
	e, ok := c.Entries[fpath]
	if !ok {
		return nil, false
	}
	if !e.ModTime.Equal(modTime) || e.Size != size {
		return nil, false
	}
	sess := e.Session
	return &sess, true
}

func (c *sessionCache) set(fpath string, modTime time.Time, size int64, sess *types.Session) {
	c.Entries[fpath] = cacheEntry{
		ModTime: modTime,
		Size:    size,
		Session: *sess,
	}
}

func (c *sessionCache) save(validPaths map[string]bool) {
	// Prune entries for files that no longer exist
	for k := range c.Entries {
		if !validPaths[k] {
			delete(c.Entries, k)
		}
	}

	p := cachePath()
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0644)
}
