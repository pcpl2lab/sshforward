package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CacheTTL is how long a release lookup is reused before asking GitHub again.
const CacheTTL = 24 * time.Hour

// CacheFileName is the cache's name inside the sshforward config directory.
const CacheFileName = "update-check.json"

// Cache remembers the last release lookup so that a check costs one request a
// day rather than one per command.
type Cache struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
}

// Fresh reports whether the cache may be used instead of asking GitHub.
// A timestamp in the future is rejected: it means the clock moved, not that
// the answer is unusually fresh.
func (c *Cache) Fresh(ttl time.Duration) bool {
	if c == nil || c.CheckedAt.IsZero() {
		return false
	}
	age := time.Since(c.CheckedAt)
	return age >= 0 && age < ttl
}

// CachePath returns the cache location inside the given sshforward directory.
func CachePath(dir string) string {
	return filepath.Join(dir, CacheFileName)
}

// LoadCache reads the cache. A missing or damaged file yields an empty cache
// rather than an error: a cache must never be able to break the command the
// user actually ran.
func LoadCache(path string) *Cache {
	var c Cache
	data, err := os.ReadFile(path)
	if err != nil {
		return &c
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return &Cache{}
	}
	return &c
}

// SaveCache writes the cache, creating its directory if needed.
func SaveCache(path string, c *Cache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("cannot create cache directory: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal update cache: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("cannot write update cache: %w", err)
	}
	return nil
}
