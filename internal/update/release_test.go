package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const latestReleaseBody = `{
  "tag_name": "v1.4.0",
  "html_url": "https://github.com/pcpl2lab/sshforward/releases/tag/v1.4.0",
  "assets": [
    {"name": "sshforward_linux_amd64.tar.gz", "browser_download_url": "https://dl.example/sshforward_linux_amd64.tar.gz"},
    {"name": "checksums.txt", "browser_download_url": "https://dl.example/checksums.txt"}
  ]
}`

func TestClientLatest_ParsesRelease(t *testing.T) {
	var gotPath, gotAccept, gotAgent, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAccept = r.URL.Path, r.Header.Get("Accept")
		gotAgent, gotAuth = r.Header.Get("User-Agent"), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(latestReleaseBody))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward", UserAgent: "sshforward/v1.0.0", Token: "secret"}
	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest() failed: %v", err)
	}

	if rel.Version != "v1.4.0" {
		t.Errorf("got version %q, want %q", rel.Version, "v1.4.0")
	}
	if rel.URL != "https://github.com/pcpl2lab/sshforward/releases/tag/v1.4.0" {
		t.Errorf("got release URL %q, want the html_url from the payload", rel.URL)
	}
	if gotPath != "/repos/pcpl2lab/sshforward/releases/latest" {
		t.Errorf("got request path %q, want /repos/pcpl2lab/sshforward/releases/latest", gotPath)
	}
	if gotAccept != "application/vnd.github+json" {
		t.Errorf("got Accept %q, want application/vnd.github+json", gotAccept)
	}
	if gotAgent != "sshforward/v1.0.0" {
		t.Errorf("got User-Agent %q, want sshforward/v1.0.0", gotAgent)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("got Authorization %q, want Bearer secret", gotAuth)
	}
}

func TestClientLatest_OmitsAuthorizationWithoutToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(latestReleaseBody))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"}
	if _, err := c.Latest(context.Background()); err != nil {
		t.Fatalf("Latest() failed: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("got Authorization %q, want no header when no token is configured", gotAuth)
	}
}

func TestReleaseAssetURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(latestReleaseBody))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"}
	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest() failed: %v", err)
	}

	url, ok := rel.AssetURL("checksums.txt")
	if !ok {
		t.Fatalf("AssetURL(%q) not found; release has %d assets", "checksums.txt", len(rel.Assets))
	}
	if url != "https://dl.example/checksums.txt" {
		t.Errorf("got asset URL %q, want https://dl.example/checksums.txt", url)
	}

	if _, ok := rel.AssetURL("sshforward_plan9_mips.tar.gz"); ok {
		t.Error("AssetURL must report missing assets as not found")
	}
}

func TestClientLatest_HTTPErrorIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"}
	_, err := c.Latest(context.Background())
	if err == nil {
		t.Fatal("got nil error for a 403 response, want one")
	}
	// The status is what tells a user apart a rate limit from a typo in the repo.
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("got error %q, want it to mention the 403 status", err)
	}
}

func TestClientLatest_MalformedJSONIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Repo: "pcpl2lab/sshforward"}
	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("got nil error for a non-JSON response, want one")
	}
}
