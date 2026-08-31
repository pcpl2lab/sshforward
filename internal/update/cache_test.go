package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	checked := time.Now().UTC().Truncate(time.Second)

	if err := SaveCache(path, &Cache{CheckedAt: checked, LatestVersion: "v1.4.0"}); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}

	got := LoadCache(path)
	if got.LatestVersion != "v1.4.0" {
		t.Errorf("got latest version %q, want v1.4.0", got.LatestVersion)
	}
	if !got.CheckedAt.Equal(checked) {
		t.Errorf("got checked-at %v, want %v", got.CheckedAt, checked)
	}
}

func TestSaveCache_CreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "update-check.json")
	if err := SaveCache(path, &Cache{CheckedAt: time.Now(), LatestVersion: "v1.0.0"}); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("cache file was not created: %v", err)
	}
}

func TestLoadCache_MissingFileIsAnEmptyCache(t *testing.T) {
	// No cache yet is the normal state on a first run, not a failure.
	got := LoadCache(filepath.Join(t.TempDir(), "absent.json"))
	if got == nil {
		t.Fatal("LoadCache returned nil; callers must always get a usable cache")
	}
	if got.LatestVersion != "" || !got.CheckedAt.IsZero() {
		t.Errorf("got %+v, want a zero cache", got)
	}
}

func TestLoadCache_CorruptFileIsAnEmptyCache(t *testing.T) {
	// A damaged cache must never break the command the user actually ran.
	path := filepath.Join(t.TempDir(), "update-check.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}

	got := LoadCache(path)
	if got == nil {
		t.Fatal("LoadCache returned nil for a corrupt file")
	}
	if got.LatestVersion != "" {
		t.Errorf("got latest version %q from a corrupt cache, want none", got.LatestVersion)
	}
}

func TestCacheFresh(t *testing.T) {
	const ttl = 24 * time.Hour

	tests := []struct {
		name  string
		cache Cache
		want  bool
	}{
		{
			name:  "checked just now",
			cache: Cache{CheckedAt: time.Now(), LatestVersion: "v1.0.0"},
			want:  true,
		},
		{
			name:  "checked within the ttl",
			cache: Cache{CheckedAt: time.Now().Add(-23 * time.Hour), LatestVersion: "v1.0.0"},
			want:  true,
		},
		{
			name:  "checked beyond the ttl",
			cache: Cache{CheckedAt: time.Now().Add(-25 * time.Hour), LatestVersion: "v1.0.0"},
			want:  false,
		},
		{
			name:  "never checked",
			cache: Cache{},
			want:  false,
		},
		{
			name:  "clock skew from the future is not trusted",
			cache: Cache{CheckedAt: time.Now().Add(48 * time.Hour), LatestVersion: "v1.0.0"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cache.Fresh(ttl); got != tt.want {
				t.Errorf("Fresh(%v) = %v, want %v (checked at %v)", ttl, got, tt.want, tt.cache.CheckedAt)
			}
		})
	}
}
