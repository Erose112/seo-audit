//go:build integration

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func buildIntegrationBinary(t *testing.T, dir string) string {
	t.Helper()
	root := repoRoot(t)
	out := filepath.Join(dir, "seo-audit")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	ldflags := "-s -w " +
		"-X github.com/Erose112/seo-audit/internal/buildinfo.version=integration-test " +
		"-X github.com/Erose112/seo-audit/internal/buildinfo.commit=deadbeef " +
		"-X github.com/Erose112/seo-audit/internal/buildinfo.date=2026-09-15T00:00:00Z"
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", out, ".")
	cmd.Dir = root
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, outBytes)
	}
	return out
}

func runBinary(t *testing.T, bin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %v: %v", args, err)
		}
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

type mutableFixture struct {
	srv            *httptest.Server
	includeCanonical atomic.Bool
	includeTitle   atomic.Bool
}

func newMutableFixture(t *testing.T) *mutableFixture {
	t.Helper()
	f := &mutableFixture{}
	f.includeCanonical.Store(true)
	f.includeTitle.Store(true)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: seo-audit\nAllow: /\n"))
		case "/":
			w.Header().Set("Content-Type", "text/html")
			title := "Home"
			if !f.includeTitle.Load() {
				title = ""
			}
			canonical := ""
			if f.includeCanonical.Load() {
				canonical = `<link rel="canonical" href="` + f.srv.URL + `/">`
			}
			body := fmt.Sprintf(`<!DOCTYPE html><html><head><title>%s</title>`+
				`<meta name="viewport" content="width=device-width">`+
				`<meta name="description" content="A good description for the home page that is long enough.">`+
				`%s</head><body><h1>Heading</h1></body></html>`, title, canonical)
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestIntegrationExitCodePass(t *testing.T) {
	fix := newMutableFixture(t)
	dir := t.TempDir()
	bin := buildIntegrationBinary(t, dir)
	baseline := filepath.Join(dir, "latest.json")

	_, stderr, code := runBinary(t, bin,
		"crawl",
		"--url", fix.srv.URL+"/",
		"--output", "json",
		"--fail-below", "80",
		"--baseline", baseline,
		"--max-pages", "5",
		"--delay", "0",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if _, err := os.Stat(baseline); err != nil {
		t.Fatalf("baseline not written: %v", err)
	}
}

func TestIntegrationExitCodeScoreGate(t *testing.T) {
	fix := newMutableFixture(t)
	fix.includeTitle.Store(false)
	dir := t.TempDir()
	bin := buildIntegrationBinary(t, dir)

	stdout, stderr, code := runBinary(t, bin,
		"crawl",
		"--url", fix.srv.URL+"/",
		"--output", "json",
		"--fail-below", "100",
		"--max-pages", "5",
		"--delay", "0",
	)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	if stdout == "" {
		t.Fatal("expected JSON report on stdout even on gate failure")
	}
}

func TestIntegrationExitCodeRegression(t *testing.T) {
	fix := newMutableFixture(t)
	dir := t.TempDir()
	bin := buildIntegrationBinary(t, dir)
	baseline := filepath.Join(dir, "latest.json")

	_, _, code := runBinary(t, bin,
		"crawl",
		"--url", fix.srv.URL+"/",
		"--output", "json",
		"--fail-below", "80",
		"--baseline", baseline,
		"--max-pages", "5",
		"--delay", "0",
	)
	if code != 0 {
		t.Fatalf("run 1 exit code = %d, want 0", code)
	}

	fix.includeCanonical.Store(false)

	stdout, stderr, code := runBinary(t, bin,
		"crawl",
		"--url", fix.srv.URL+"/",
		"--output", "json",
		"--fail-below", "80",
		"--baseline", baseline,
		"--max-pages", "5",
		"--delay", "0",
	)
	if code != 1 {
		t.Fatalf("run 2 exit code = %d, want 1; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "SEO REGRESSION") {
		t.Fatalf("stderr missing regression block: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	if payload["schema_version"] != float64(1) {
		t.Errorf("schema_version = %v, want 1", payload["schema_version"])
	}
	if _, err := os.Stat(baseline + ".tmp"); err == nil {
		t.Fatal("leftover .tmp baseline file")
	}
}

func TestIntegrationExitCodeCrawlError(t *testing.T) {
	dir := t.TempDir()
	bin := buildIntegrationBinary(t, dir)

	_, stderr, code := runBinary(t, bin,
		"crawl",
		"--url", "http://127.0.0.1:1/",
		"--output", "json",
		"--max-pages", "1",
		"--delay", "0",
	)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "crawl error:") {
		t.Fatalf("stderr missing crawl error: %q", stderr)
	}
}

func TestIntegrationVersion(t *testing.T) {
	dir := t.TempDir()
	bin := buildIntegrationBinary(t, dir)

	stdout, _, code := runBinary(t, bin, "--version")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "integration-test") {
		t.Fatalf("--version output = %q, want integration-test version", stdout)
	}
}
