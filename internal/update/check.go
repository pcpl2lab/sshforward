package update

import (
	"context"
	"time"
)

// ReleasesURL is the page a user is pointed at when an update exists.
const ReleasesURL = "https://github.com/" + Repo + "/releases/latest"

// CheckOptions configures a release lookup.
type CheckOptions struct {
	// CurrentVersion is the running binary's version.
	CurrentVersion string
	// Dir is the sshforward directory holding the cache.
	Dir string
	// Client reads the release; nil means a default one.
	Client *Client
	// TTL overrides CacheTTL.
	TTL time.Duration
	// Force asks GitHub even when the cache is still fresh.
	Force bool
}

// Result is the outcome of a check.
type Result struct {
	// Latest is the newest published version, as tagged.
	Latest string
	// Available reports whether Latest is worth upgrading to.
	Available bool
	// FromCache reports whether the answer was reused rather than fetched.
	FromCache bool
	// Release is the fetched release, carrying the assets a self-update needs.
	// It is nil for a cached answer, which remembers a version but no assets.
	Release *Release
}

// Check reports whether a newer release exists, reusing the cached answer while
// it is fresh so that a per-command check costs one request a day.
//
// The cache is only written after a successful lookup: a failed check must not
// replace the last good answer, nor reset the timer that keeps checks rare.
func Check(ctx context.Context, opts CheckOptions) (*Result, error) {
	ttl := opts.TTL
	if ttl == 0 {
		ttl = CacheTTL
	}
	cachePath := CachePath(opts.Dir)

	if !opts.Force {
		if cached := LoadCache(cachePath); cached.Fresh(ttl) {
			return &Result{
				Latest:    cached.LatestVersion,
				Available: IsNewer(opts.CurrentVersion, cached.LatestVersion),
				FromCache: true,
			}, nil
		}
	}

	client := opts.Client
	if client == nil {
		client = &Client{UserAgent: "sshforward/" + opts.CurrentVersion}
	}
	rel, err := client.Latest(ctx)
	if err != nil {
		return nil, err
	}

	// A cache write failure is not worth failing the check over — the only
	// cost is asking again next time.
	_ = SaveCache(cachePath, &Cache{CheckedAt: time.Now().UTC(), LatestVersion: rel.Version})

	return &Result{
		Latest:    rel.Version,
		Available: IsNewer(opts.CurrentVersion, rel.Version),
		Release:   rel,
	}, nil
}
