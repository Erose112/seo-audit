package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Erose112/seo-audit/internal/config"
	"github.com/spf13/cobra"
)

func newCrawlReportServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: seo-audit\nAllow: /\n"))
		case "/":
			w.Header().Set("Content-Type", "text/html")
			body := `<!DOCTYPE html><html><head><title>Home</title><meta name="viewport" content="width=device-width"><meta name="description" content="A good description for the home page that is long enough."><link rel="canonical" href="` + "http://" + r.Host + `/"></head><body><h1>Heading</h1></body></html>`
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCrawlJSONOutput(t *testing.T) {
	srv := newCrawlReportServer(t)

	var stdout bytes.Buffer
	cfg := config.CrawlConfig{
		URL:      srv.URL + "/",
		MaxPages: 10,
		MaxDepth: 2,
		Delay:    0,
		Output:   "json",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rep, err := runCrawl(ctx, cfg)
	if err != nil {
		t.Fatalf("runCrawl: %v", err)
	}
	if err := rep.WriteJSON(&stdout); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	raw := stdout.Bytes()
	if len(raw) == 0 {
		t.Fatal("stdout is empty")
	}
	if raw[len(raw)-1] != '\n' {
		t.Fatalf("stdout missing trailing newline")
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, raw)
	}
	if payload["schema_version"] != float64(1) {
		t.Errorf("schema_version = %v, want 1", payload["schema_version"])
	}
	if _, ok := payload["site_issues"]; !ok {
		t.Error("missing site_issues key")
	}
}

func TestCrawlCorruptBaseline(t *testing.T) {
	old := crawlCfg
	t.Cleanup(func() { crawlCfg = old })

	dir := t.TempDir()
	baseline := dir + "/baseline.json"
	if err := os.WriteFile(baseline, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	crawlCfg = config.CrawlConfig{
		URL:      "https://example.com",
		Output:   "json",
		Baseline: baseline,
	}

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := crawlCmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for corrupt baseline")
	}
}

func TestCrawlInvalidOutput(t *testing.T) {
	old := crawlCfg
	t.Cleanup(func() { crawlCfg = old })
	crawlCfg = config.CrawlConfig{
		URL:    "https://example.com",
		Output: "xml",
	}

	cmd := &cobra.Command{}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	err := crawlCmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid --output")
	}
	if !strings.Contains(err.Error(), "invalid --output") {
		t.Errorf("error = %q, want invalid --output message", err)
	}
}
