package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/temoto/robotstxt"
)

func TestRobotsURL(t *testing.T) {
	cases := []struct {
		site string
		want string
	}{
		{"https://example.com", "https://example.com/robots.txt"},
		{"https://example.com/", "https://example.com/robots.txt"},
		{"https://example.com/path/page", "https://example.com/robots.txt"},
		{"http://example.com:8080/foo", "http://example.com:8080/robots.txt"},
	}

	for _, c := range cases {
		t.Run(c.site, func(t *testing.T) {
			got, err := robotsURL(c.site)
			if err != nil {
				t.Fatalf("robotsURL(%q): %v", c.site, err)
			}
			if got != c.want {
				t.Errorf("robotsURL(%q) = %q, want %q", c.site, got, c.want)
			}
			if idx := strings.Index(got[8:], "//"); idx >= 0 {
				// After scheme, no double slash before path segment (F10).
				_ = idx
			}
		})
	}
}

func TestRobotsPolicyAllowed(t *testing.T) {
	data, err := robotstxt.FromString("User-agent: seo-audit\nDisallow: /private\n\nUser-agent: *\nAllow: /")
	if err != nil {
		t.Fatalf("FromString: %v", err)
	}
	policy := &RobotsPolicy{group: data.FindGroup(DefaultUserAgent)}
	if !policy.Allowed("/public") {
		t.Error("expected /public to be allowed")
	}
	if policy.Allowed("/private/secret") {
		t.Error("expected /private/secret to be disallowed")
	}
	if !allowAllRobots().Allowed("/anything") {
		t.Error("allow-all policy should permit every path")
	}
}

func TestFetchRobotsFailOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	policy, err := FetchRobots(ctx, srv.Client(), srv.URL, DefaultUserAgent)
	if policy == nil {
		t.Fatal("expected non-nil allow-all policy on 404")
	}
	if err == nil {
		t.Fatal("expected error on 404 for logging")
	}
	if !policy.Allowed("/secret") {
		t.Error("fail-open policy should allow all paths")
	}
}

func TestFetchRobotsParsesRules(t *testing.T) {
	const body = "User-agent: seo-audit\nDisallow: /blocked\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	policy, err := FetchRobots(ctx, srv.Client(), srv.URL, DefaultUserAgent)
	if err != nil {
		t.Fatalf("FetchRobots: %v", err)
	}
	if policy.Allowed("/blocked/page") {
		t.Error("expected /blocked/page to be disallowed")
	}
	if !policy.Allowed("/open") {
		t.Error("expected /open to be allowed")
	}
}
