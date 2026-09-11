package crawler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testHTML = `<!DOCTYPE html><html><head><title>T</title></head><body><p>hello</p></body></html>`

// fastRetry keeps the escalation shape of the real schedule but on a
// millisecond scale, so retry behavior is testable without slow tests.
func fastRetry() RetryConfig {
	return RetryConfig{
		MaxRetries:        2,
		BaseTimeout:       2 * time.Second,
		TimeoutMultiplier: 1.5,
		BaseBackoff:       5 * time.Millisecond,
		MaxTotalPerPage:   10 * time.Second,
	}
}

func newTestFetcher(t *testing.T, cfg Config, handler http.HandlerFunc) (*Fetcher, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	f, err := NewFetcher(NewClient(cfg), cfg)
	if err != nil {
		t.Fatalf("NewFetcher: %v", err)
	}
	return f, srv
}

func fetchErrorFrom(t *testing.T, err error) *FetchError {
	t.Helper()
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("error %v is not a *FetchError", err)
	}
	return fe
}

func sleepingHandler(d time.Duration, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(d):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}
}

func TestFetchOnceSuccess(t *testing.T) {
	f, srv := newTestFetcher(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "seo-audit/") {
			t.Errorf("User-Agent = %q, want the seo-audit identifier", ua)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(testHTML))
	})

	page, err := f.fetchOnce(context.Background(), srv.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("fetchOnce: %v", err)
	}
	if page.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", page.StatusCode)
	}
	if string(page.Body) != testHTML {
		t.Errorf("Body = %q, want the served document", page.Body)
	}
	if page.Truncated {
		t.Error("Truncated = true, want false for a small body")
	}
	if page.URL != srv.URL || page.FinalURL != srv.URL {
		t.Errorf("URL/FinalURL = %q/%q, want %q", page.URL, page.FinalURL, srv.URL)
	}
	if page.Duration <= 0 {
		t.Error("Duration = 0, want a measured fetch duration")
	}
}

func TestFetchOnceSkipsNonHTMLBody(t *testing.T) {
	f, srv := newTestFetcher(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7 binary junk"))
	})

	page, err := f.fetchOnce(context.Background(), srv.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("fetchOnce: %v", err)
	}
	if page.Body != nil {
		t.Errorf("Body = %q, want nil for a non-HTML content type", page.Body)
	}
	if page.ContentType != "application/pdf" {
		t.Errorf("ContentType = %q, want application/pdf", page.ContentType)
	}
}

func TestFetchOnceBodySizeLimit(t *testing.T) {
	const limit = 512

	cases := []struct {
		name          string
		bodySize      int
		wantLen       int
		wantTruncated bool
	}{
		{"under the limit", limit - 1, limit - 1, false},
		{"exactly at the limit", limit, limit, false},
		{"over the limit", limit * 3, limit, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.MaxBodySize = limit

			f, srv := newTestFetcher(t, cfg, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(strings.Repeat("a", c.bodySize)))
			})

			page, err := f.fetchOnce(context.Background(), srv.URL, 2*time.Second)
			if err != nil {
				t.Fatalf("fetchOnce: %v", err)
			}
			if len(page.Body) != c.wantLen {
				t.Errorf("len(Body) = %d, want %d", len(page.Body), c.wantLen)
			}
			if page.Truncated != c.wantTruncated {
				t.Errorf("Truncated = %v, want %v", page.Truncated, c.wantTruncated)
			}
		})
	}
}

func TestFetchOnceHTTPStatusError(t *testing.T) {
	f, srv := newTestFetcher(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	page, err := f.fetchOnce(context.Background(), srv.URL, 2*time.Second)
	if page != nil {
		t.Errorf("page = %+v, want nil alongside an error", page)
	}
	fe := fetchErrorFrom(t, err)
	if fe.Kind != ErrKindHTTPStatus {
		t.Errorf("Kind = %v, want %v", fe.Kind, ErrKindHTTPStatus)
	}
	if fe.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", fe.StatusCode)
	}
}

func TestFetchWithRetryRecoversFromTransientFailures(t *testing.T) {
	var requests atomic.Int32

	f, srv := newTestFetcher(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) <= 2 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(testHTML))
	})

	page, err := f.FetchWithRetry(context.Background(), srv.URL, fastRetry())
	if err != nil {
		t.Fatalf("FetchWithRetry: %v", err)
	}
	if string(page.Body) != testHTML {
		t.Errorf("Body = %q, want the served document", page.Body)
	}
	if got := requests.Load(); got != 3 {
		t.Errorf("requests = %d, want 3 (two failures then a success)", got)
	}
}

func TestFetchWithRetryStopsOnNonRetryableStatus(t *testing.T) {
	var requests atomic.Int32

	f, srv := newTestFetcher(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	})

	_, err := f.FetchWithRetry(context.Background(), srv.URL, fastRetry())
	fe := fetchErrorFrom(t, err)
	if fe.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", fe.StatusCode)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1 — a 404 is a finding, not a transient failure", got)
	}
}

func TestFetchWithRetryExhaustsRetries(t *testing.T) {
	var requests atomic.Int32

	f, srv := newTestFetcher(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	_, err := f.FetchWithRetry(context.Background(), srv.URL, fastRetry())
	fe := fetchErrorFrom(t, err)
	if fe.Kind != ErrKindHTTPStatus || fe.StatusCode != http.StatusInternalServerError {
		t.Errorf("error = %v, want a 500 http_status error", fe)
	}
	if got := requests.Load(); got != 3 {
		t.Errorf("requests = %d, want 3 (one attempt plus two retries)", got)
	}
}

func TestFetchWithRetryEscalatesPerAttemptTimeout(t *testing.T) {
	const serverDelay = 150 * time.Millisecond

	var requests atomic.Int32
	f, srv := newTestFetcher(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		sleepingHandler(serverDelay, testHTML)(w, r)
	})

	// Attempt 1 gets 60ms and must time out; attempt 2 gets 240ms and must
	// succeed against the same 150ms server. A fixed per-attempt timeout would
	// fail every attempt here.
	rc := RetryConfig{
		MaxRetries:        2,
		BaseTimeout:       60 * time.Millisecond,
		TimeoutMultiplier: 4,
		BaseBackoff:       5 * time.Millisecond,
		MaxTotalPerPage:   10 * time.Second,
	}

	page, err := f.FetchWithRetry(context.Background(), srv.URL, rc)
	if err != nil {
		t.Fatalf("FetchWithRetry: %v", err)
	}
	if string(page.Body) != testHTML {
		t.Errorf("Body = %q, want the served document", page.Body)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("requests = %d, want 2 — the second attempt should have a longer timeout", got)
	}
}

func TestFetchWithRetryEnforcesPerPageCeiling(t *testing.T) {
	f, srv := newTestFetcher(t, DefaultConfig(), sleepingHandler(10*time.Second, testHTML))

	rc := RetryConfig{
		MaxRetries:        20,
		BaseTimeout:       40 * time.Millisecond,
		TimeoutMultiplier: 1,
		BaseBackoff:       10 * time.Millisecond,
		MaxTotalPerPage:   200 * time.Millisecond,
	}

	start := time.Now()
	_, err := f.FetchWithRetry(context.Background(), srv.URL, rc)
	elapsed := time.Since(start)

	fe := fetchErrorFrom(t, err)
	if fe.Kind != ErrKindTimeout {
		t.Errorf("Kind = %v, want %v — an exhausted page ceiling is page-level", fe.Kind, ErrKindTimeout)
	}
	// The ceiling must bound the whole schedule, not just one attempt; without
	// it, 21 attempts of 40ms plus backoff would run well past a second.
	if elapsed > time.Second {
		t.Errorf("elapsed = %v, want the %v ceiling to stop the schedule", elapsed, rc.MaxTotalPerPage)
	}
}

func TestFetchWithRetryTreatsDeadParentAsSystemic(t *testing.T) {
	var requests atomic.Int32
	f, srv := newTestFetcher(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(testHTML))
	})

	t.Run("canceled parent", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := f.FetchWithRetry(ctx, srv.URL, fastRetry())
		if fe := fetchErrorFrom(t, err); fe.Kind != ErrKindCanceled {
			t.Errorf("Kind = %v, want %v", fe.Kind, ErrKindCanceled)
		}
		if got := requests.Load(); got != 0 {
			t.Errorf("requests = %d, want 0 — a dead root context should not dispatch", got)
		}
	})

	t.Run("expired root deadline", func(t *testing.T) {
		// A root deadline and a per-page ceiling both surface as
		// context.DeadlineExceeded, but only the root one is systemic.
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()
		time.Sleep(time.Millisecond)

		_, err := f.FetchWithRetry(ctx, srv.URL, fastRetry())
		if fe := fetchErrorFrom(t, err); fe.Kind != ErrKindCanceled {
			t.Errorf("Kind = %v, want %v — root budget expiry is systemic, not a page timeout", fe.Kind, ErrKindCanceled)
		}
	})
}

func TestFetchWithRetryDoesNotRetryRedirectLoops(t *testing.T) {
	var requests atomic.Int32

	f, srv := newTestFetcher(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "/loop", http.StatusFound)
	})

	_, err := f.FetchWithRetry(context.Background(), srv.URL, fastRetry())
	fe := fetchErrorFrom(t, err)
	if fe.Kind != ErrKindPermanent {
		t.Errorf("Kind = %v, want %v", fe.Kind, ErrKindPermanent)
	}
	if got := requests.Load(); got > maxRedirects+1 {
		t.Errorf("requests = %d, want at most %d — a redirect loop must not be retried",
			got, maxRedirects+1)
	}
}

func TestFetchFollowsRedirectsAndReportsFinalURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/end", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(testHTML))
	})

	f, srv := newTestFetcher(t, DefaultConfig(), mux.ServeHTTP)

	page, err := f.FetchWithRetry(context.Background(), srv.URL+"/start", fastRetry())
	if err != nil {
		t.Fatalf("FetchWithRetry: %v", err)
	}
	if page.URL != srv.URL+"/start" {
		t.Errorf("URL = %q, want the requested URL", page.URL)
	}
	if page.FinalURL != srv.URL+"/end" {
		t.Errorf("FinalURL = %q, want the post-redirect URL", page.FinalURL)
	}
}

func TestIsHTML(t *testing.T) {
	cases := []struct {
		contentType string
		want        bool
	}{
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"application/xhtml+xml", true},
		{"", true},
		{"   ", true},
		{"application/pdf", false},
		{"image/png", false},
		{"application/json", false},
		{"not a media type;;;", false},
	}

	for _, c := range cases {
		t.Run(c.contentType, func(t *testing.T) {
			if got := IsHTML(c.contentType); got != c.want {
				t.Errorf("IsHTML(%q) = %v, want %v", c.contentType, got, c.want)
			}
		})
	}
}

func TestNewFetcherRejectsZeroRetryConfig(t *testing.T) {
	cfg := Config{
		MaxBodySize: 1024,
		UserAgent:   "test",
		// Retry left zero — the failure mode this guard exists to catch.
	}
	f, err := NewFetcher(http.DefaultClient, cfg)
	if f != nil {
		t.Errorf("NewFetcher: got fetcher %v, want nil", f)
	}
	if err == nil || !strings.Contains(err.Error(), "BaseTimeout must be > 0") {
		t.Fatalf("NewFetcher: want BaseTimeout validation error, got %v", err)
	}
}

func TestFetchWithRetryRejectsZeroRetryConfig(t *testing.T) {
	// NewFetcher gets a valid schedule; the call site still passes a zero rc.
	f, srv := newTestFetcher(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be reached when RetryConfig is invalid")
	})

	_, err := f.FetchWithRetry(context.Background(), srv.URL, RetryConfig{})
	if err == nil || !strings.Contains(err.Error(), "BaseTimeout must be > 0") {
		t.Fatalf("FetchWithRetry: want BaseTimeout validation error, got %v", err)
	}
}
