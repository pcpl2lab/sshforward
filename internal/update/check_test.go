package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// countingServer serves a fixed release and records how often it was asked.
func countingServer(t *testing.T, tag string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://example/releases","assets":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestCheck_QueriesGitHubAndCachesTheAnswer(t *testing.T) {
	srv, hits := countingServer(t, "v1.4.0")
	dir := t.TempDir()

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "v1.0.0",
		Dir:            dir,
		Client:         &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"},
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !res.Available {
		t.Errorf("got Available=false for v1.0.0 against v1.4.0, want true")
	}
	if res.Latest != "v1.4.0" {
		t.Errorf("got latest %q, want v1.4.0", res.Latest)
	}
	if hits.Load() != 1 {
		t.Errorf("got %d requests, want exactly 1", hits.Load())
	}

	cached := LoadCache(CachePath(dir))
	if cached.LatestVersion != "v1.4.0" {
		t.Errorf("got cached version %q, want the answer to be remembered", cached.LatestVersion)
	}
}

func TestCheck_FreshCacheSkipsTheNetwork(t *testing.T) {
	srv, hits := countingServer(t, "v1.4.0")
	dir := t.TempDir()

	if err := SaveCache(CachePath(dir), &Cache{CheckedAt: time.Now(), LatestVersion: "v1.4.0"}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "v1.0.0",
		Dir:            dir,
		Client:         &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"},
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if res.Latest != "v1.4.0" || !res.Available {
		t.Errorf("got %+v, want the cached v1.4.0 reported as available", res)
	}
	// A check that runs on every command must not cost a request every time.
	if hits.Load() != 0 {
		t.Errorf("got %d requests, want none while the cache is fresh", hits.Load())
	}
}

func TestCheck_StaleCacheRefreshes(t *testing.T) {
	srv, hits := countingServer(t, "v2.0.0")
	dir := t.TempDir()

	if err := SaveCache(CachePath(dir), &Cache{CheckedAt: time.Now().Add(-48 * time.Hour), LatestVersion: "v1.4.0"}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "v1.0.0",
		Dir:            dir,
		Client:         &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"},
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if res.Latest != "v2.0.0" {
		t.Errorf("got latest %q, want the refreshed v2.0.0", res.Latest)
	}
	if hits.Load() != 1 {
		t.Errorf("got %d requests, want exactly 1 once the cache went stale", hits.Load())
	}
}

func TestCheck_ForceIgnoresAFreshCache(t *testing.T) {
	srv, hits := countingServer(t, "v2.0.0")
	dir := t.TempDir()

	if err := SaveCache(CachePath(dir), &Cache{CheckedAt: time.Now(), LatestVersion: "v1.4.0"}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "v1.0.0",
		Dir:            dir,
		Force:          true,
		Client:         &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"},
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if res.Latest != "v2.0.0" {
		t.Errorf("got latest %q, want Force to bypass the cache and return v2.0.0", res.Latest)
	}
	if hits.Load() != 1 {
		t.Errorf("got %d requests, want 1 when the check is forced", hits.Load())
	}
}

func TestCheck_UpToDateReportsNoUpdate(t *testing.T) {
	srv, _ := countingServer(t, "v1.4.0")

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "v1.4.0",
		Dir:            t.TempDir(),
		Client:         &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"},
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if res.Available {
		t.Errorf("got Available=true while already on the latest version %q", res.Latest)
	}
}

func TestCheck_NetworkFailureIsReportedAndLeavesTheCacheAlone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	seeded := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)
	if err := SaveCache(CachePath(dir), &Cache{CheckedAt: seeded, LatestVersion: "v1.4.0"}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	_, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "v1.0.0",
		Dir:            dir,
		Client:         &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"},
	})
	if err == nil {
		t.Fatal("got nil error for a failing server, want one")
	}

	// A failed lookup must not overwrite the last good answer with nothing.
	cached := LoadCache(CachePath(dir))
	if cached.LatestVersion != "v1.4.0" || !cached.CheckedAt.Equal(seeded) {
		t.Errorf("cache changed to %+v after a failed check, want it left untouched", cached)
	}
}

func TestCheck_DevBuildNeverReportsAnUpdate(t *testing.T) {
	srv, _ := countingServer(t, "v9.9.9")

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "dev",
		Dir:            t.TempDir(),
		Client:         &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"},
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if res.Available {
		t.Error("a locally built binary must never be told it is out of date")
	}
}

func TestCheck_NetworkAnswerCarriesTheRelease(t *testing.T) {
	// `sshforward update` needs the asset list to install; a Result without it
	// would force a second request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(latestReleaseBody))
	}))
	defer srv.Close()

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "v1.0.0",
		Dir:            t.TempDir(),
		Client:         &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"},
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if res.Release == nil {
		t.Fatal("got no Release on a network answer, want the fetched release")
	}
	if _, ok := res.Release.AssetURL(ChecksumsFileName); !ok {
		t.Error("the carried release lost its assets")
	}
}

func TestCheck_CachedAnswerCarriesNoRelease(t *testing.T) {
	// The cache stores a version, not an asset list; pretending otherwise
	// would hand callers stale download URLs.
	dir := t.TempDir()
	if err := SaveCache(CachePath(dir), &Cache{CheckedAt: time.Now(), LatestVersion: "v1.4.0"}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "v1.0.0",
		Dir:            dir,
		Client:         &Client{BaseURL: "http://127.0.0.1:1", Repo: "pcpl2lab/sshforward"},
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if res.Release != nil {
		t.Error("got a Release from the cache, want nil")
	}
}
