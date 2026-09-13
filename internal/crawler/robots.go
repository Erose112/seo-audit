package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/temoto/robotstxt"
)

const robotsFetchTimeout = 10 * time.Second

// RobotsPolicy wraps parsed robots.txt rules. Allowed always succeeds on a
// nil receiver; production code should still receive a non-nil policy.
type RobotsPolicy struct {
	group    *robotstxt.Group
	allowAll bool
}

// Allowed reports whether path may be fetched per robots.txt. path must be the
// resolved request path (including query), not a Normalize dedup key (D4).
func (p *RobotsPolicy) Allowed(path string) bool {
	if p == nil || p.allowAll || p.group == nil {
		return true
	}
	return p.group.Test(path)
}

// FetchRobots loads and parses robots.txt for siteURL. On any failure it
// returns a non-nil allow-all policy plus an error for logging. Does not
// use FromStatusAndBytes: its 5xx disallow-all semantics conflict with
// fail-open (arch §7 Rule 2).
func FetchRobots(ctx context.Context, client *http.Client, siteURL, userAgent string) (*RobotsPolicy, error) {
	robotsURL, err := robotsURL(siteURL)
	if err != nil {
		return allowAllRobots(), fmt.Errorf("robots URL for %q: %w", siteURL, err)
	}

	ctx, cancel := context.WithTimeout(ctx, robotsFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return allowAllRobots(), err
	}
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return allowAllRobots(), fmt.Errorf("fetch %s: %w", robotsURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return allowAllRobots(), fmt.Errorf("fetch %s: status %d", robotsURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return allowAllRobots(), fmt.Errorf("read %s: %w", robotsURL, err)
	}

	data, err := robotstxt.FromBytes(body)
	if err != nil {
		return allowAllRobots(), fmt.Errorf("parse %s: %w", robotsURL, err)
	}

	group := data.FindGroup(userAgent)
	if group == nil {
		return allowAllRobots(), nil
	}
	return &RobotsPolicy{group: group}, nil
}

func allowAllRobots() *RobotsPolicy {
	return &RobotsPolicy{allowAll: true}
}

func robotsURL(siteURL string) (string, error) {
	base, err := url.Parse(siteURL)
	if err != nil {
		return "", err
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("URL %q is not absolute", siteURL)
	}
	ref, err := url.Parse("/robots.txt")
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

// robotsPath returns the path+query form robots.txt matchers expect.
func robotsPath(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	return path, nil
}
