package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the GitHub REST API root.
const DefaultBaseURL = "https://api.github.com"

// Repo is the repository releases are read from, in "owner/name" form.
const Repo = "pcpl2lab/sshforward"

// maxBodySize caps how much of a response we are willing to read. A release
// payload is a few kilobytes; anything larger is a misdirected request.
const maxBodySize = 1 << 20

// Asset is one file attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Release describes the newest published release of the project.
type Release struct {
	Version string  `json:"tag_name"`
	URL     string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// AssetURL returns the download URL of the asset with the given name.
func (r *Release) AssetURL(name string) (string, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL, true
		}
	}
	return "", false
}

// Client reads release information from a GitHub-compatible API.
type Client struct {
	// HTTP is the client used for requests. A zero value means a client with
	// DefaultTimeout.
	HTTP *http.Client
	// BaseURL is the API root; empty means DefaultBaseURL.
	BaseURL string
	// Repo is "owner/name"; empty means Repo.
	Repo string
	// UserAgent identifies sshforward to GitHub, which requires one.
	UserAgent string
	// Token, when set, raises the anonymous rate limit. Optional.
	Token string
}

// DefaultTimeout bounds a release lookup. The check is a convenience, never
// something a user should wait on.
const DefaultTimeout = 5 * time.Second

// Latest returns the most recent published release.
func (c *Client) Latest(ctx context.Context) (*Release, error) {
	base := strings.TrimSuffix(c.baseURL(), "/")
	url := fmt.Sprintf("%s/repos/%s/releases/latest", base, c.repo())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %w", base, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("cannot read release response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The status separates a rate limit from a missing repository, so keep it.
		return nil, fmt.Errorf("release lookup failed with HTTP %d: %s", resp.StatusCode, summarize(body))
	}

	var rel Release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("cannot parse release response: %w", err)
	}
	if rel.Version == "" {
		return nil, fmt.Errorf("release response carries no tag_name")
	}
	return &rel, nil
}

func (c *Client) baseURL() string {
	if c.BaseURL == "" {
		return DefaultBaseURL
	}
	return c.BaseURL
}

func (c *Client) repo() string {
	if c.Repo == "" {
		return Repo
	}
	return c.Repo
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: DefaultTimeout}
}

// summarize trims a response body down to something fit for one error line.
func summarize(body []byte) string {
	s := strings.TrimSpace(string(body))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	if s == "" {
		return "(empty response)"
	}
	return s
}
