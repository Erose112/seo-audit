package checks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	json "encoding/json/v2"

	"github.com/Erose112/seo-audit/internal/crawler"
	"github.com/Erose112/seo-audit/internal/parser"
)

func page(url, title string, links ...parser.Link) crawler.CrawledPage {
	return crawler.CrawledPage{
		URL: url,
		Data: parser.PageData{
			URL:   url,
			Title: title,
			Links: links,
		},
	}
}

func link(href string, internal bool) parser.Link {
	return parser.Link{Href: href, Internal: internal}
}

func TestFindDuplicateTitles(t *testing.T) {
	base := "https://example.com"

	cases := []struct {
		name  string
		pages []crawler.CrawledPage
		want  map[string][]string
	}{
		{
			name: "no shared titles",
			pages: []crawler.CrawledPage{
				page(base+"/a", "Title A"),
				page(base+"/b", "Title B"),
			},
			want: map[string][]string{},
		},
		{
			name: "two pages share a title",
			pages: []crawler.CrawledPage{
				page(base+"/a", "Same Title"),
				page(base+"/b", "Same Title"),
			},
			want: map[string][]string{
				"Same Title": {base + "/a", base + "/b"},
			},
		},
		{
			name: "two groups of duplicates",
			pages: []crawler.CrawledPage{
				page(base+"/a", "Group One"),
				page(base+"/b", "Group One"),
				page(base+"/c", "Group Two"),
				page(base+"/d", "Group Two"),
				page(base+"/e", "Unique"),
			},
			want: map[string][]string{
				"Group One": {base + "/a", base + "/b"},
				"Group Two": {base + "/c", base + "/d"},
			},
		},
		{
			name: "singleton not present",
			pages: []crawler.CrawledPage{
				page(base+"/only", "Only One"),
			},
			want: map[string][]string{},
		},
		{
			name: "empty and whitespace titles not grouped",
			pages: []crawler.CrawledPage{
				page(base+"/a", ""),
				page(base+"/b", "   "),
				page(base+"/c", "\t"),
			},
			want: map[string][]string{},
		},
		{
			name: "trimmed titles treated as duplicates",
			pages: []crawler.CrawledPage{
				page(base+"/a", "  Shared  "),
				page(base+"/b", "Shared"),
			},
			want: map[string][]string{
				"Shared": {base + "/a", base + "/b"},
			},
		},
		{
			name: "case-sensitive no match",
			pages: []crawler.CrawledPage{
				page(base+"/a", "Title"),
				page(base+"/b", "title"),
			},
			want: map[string][]string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindDuplicateTitles(tc.pages)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindDuplicateTitles() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestFindDuplicateTitlesDeterministic(t *testing.T) {
	pages := []crawler.CrawledPage{
		page("https://example.com/a", "Dup"),
		page("https://example.com/b", "Dup"),
	}
	first := FindDuplicateTitles(pages)
	second := FindDuplicateTitles(pages)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic output: first=%#v second=%#v", first, second)
	}
}

func TestFindBrokenLinks(t *testing.T) {
	base := "https://example.com"

	cases := []struct {
		name        string
		pages       []crawler.CrawledPage
		crawlErrors []crawler.CrawlError
		want        []BrokenLink
	}{
		{
			name: "404 flagged with http status",
			pages: []crawler.CrawledPage{
				page(base+"/", "root", link(base+"/missing", true)),
			},
			crawlErrors: []crawler.CrawlError{
				{URL: base + "/missing", Kind: crawler.ErrKindHTTPStatus, StatusCode: 404, Message: "404 Not Found"},
			},
			want: []BrokenLink{{
				SourceURL: base + "/", TargetURL: base + "/missing",
				Kind: crawler.ErrKindHTTPStatus, StatusCode: 404, Message: "404 Not Found",
			}},
		},
		{
			name: "connection error flagged",
			pages: []crawler.CrawledPage{
				page(base+"/", "root", link(base+"/dead", true)),
			},
			crawlErrors: []crawler.CrawlError{
				{URL: base + "/dead", Kind: crawler.ErrKindConnection, Message: "connection refused"},
			},
			want: []BrokenLink{{
				SourceURL: base + "/", TargetURL: base + "/dead",
				Kind: crawler.ErrKindConnection, Message: "connection refused",
			}},
		},
		{
			name: "parse failure with message",
			pages: []crawler.CrawledPage{
				page(base+"/", "root", link(base+"/json", true)),
			},
			crawlErrors: []crawler.CrawlError{
				{URL: base + "/json", Kind: crawler.ErrKindParseFailure, Message: `content type "application/json" is not HTML`},
			},
			want: []BrokenLink{{
				SourceURL: base + "/", TargetURL: base + "/json",
				Kind: crawler.ErrKindParseFailure, Message: `content type "application/json" is not HTML`,
			}},
		},
		{
			name: "uncrawled target not flagged",
			pages: []crawler.CrawledPage{
				page(base+"/", "root", link(base+"/never-tried", true)),
			},
			crawlErrors: nil,
			want:        nil,
		},
		{
			name: "robots-disallowed not flagged",
			pages: []crawler.CrawledPage{
				page(base+"/", "root", link(base+"/private", true)),
			},
			crawlErrors: nil,
			want:        nil,
		},
		{
			name: "fragment matches normalized error URL",
			pages: []crawler.CrawledPage{
				page(base+"/", "root", link(base+"/broken#section", true)),
			},
			crawlErrors: []crawler.CrawlError{
				{URL: base + "/broken", Kind: crawler.ErrKindHTTPStatus, StatusCode: 404},
			},
			want: []BrokenLink{{
				SourceURL: base + "/", TargetURL: base + "/broken",
				Kind: crawler.ErrKindHTTPStatus, StatusCode: 404,
			}},
		},
		{
			name: "trailing slash mismatch does not match",
			pages: []crawler.CrawledPage{
				page(base+"/", "root", link(base+"/page/", true)),
			},
			crawlErrors: []crawler.CrawlError{
				{URL: base + "/page", Kind: crawler.ErrKindHTTPStatus, StatusCode: 404},
			},
			want: nil,
		},
		{
			name: "ignores wrong Internal true on external link",
			pages: []crawler.CrawledPage{
				page(base+"/", "root", link("https://other.example/missing", true)),
			},
			crawlErrors: []crawler.CrawlError{
				{URL: "https://other.example/missing", Kind: crawler.ErrKindHTTPStatus, StatusCode: 404},
			},
			want: nil,
		},
		{
			name: "ignores wrong Internal false on internal link",
			pages: []crawler.CrawledPage{
				page(base+"/", "root", link(base+"/broken", false)),
			},
			crawlErrors: []crawler.CrawlError{
				{URL: base + "/broken", Kind: crawler.ErrKindHTTPStatus, StatusCode: 404},
			},
			want: []BrokenLink{{
				SourceURL: base + "/", TargetURL: base + "/broken",
				Kind: crawler.ErrKindHTTPStatus, StatusCode: 404,
			}},
		},
		{
			name: "deduplicates same source and target",
			pages: []crawler.CrawledPage{
				page(base+"/", "root",
					link(base+"/broken", true),
					link(base+"/broken", true),
				),
			},
			crawlErrors: []crawler.CrawlError{
				{URL: base + "/broken", Kind: crawler.ErrKindHTTPStatus, StatusCode: 404},
			},
			want: []BrokenLink{{
				SourceURL: base + "/", TargetURL: base + "/broken",
				Kind: crawler.ErrKindHTTPStatus, StatusCode: 404,
			}},
		},
		{
			name: "external link not flagged even if error exists",
			pages: []crawler.CrawledPage{
				page(base+"/", "root", link("https://other.example/404", false)),
			},
			crawlErrors: []crawler.CrawlError{
				{URL: "https://other.example/404", Kind: crawler.ErrKindHTTPStatus, StatusCode: 404},
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindBrokenLinks(tc.pages, tc.crawlErrors)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindBrokenLinks() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestFindBrokenLinksDeterministic(t *testing.T) {
	base := "https://example.com"
	pages := []crawler.CrawledPage{
		page(base+"/", "root", link(base+"/a", true), link(base+"/b", true)),
	}
	errs := []crawler.CrawlError{
		{URL: base + "/a", Kind: crawler.ErrKindHTTPStatus, StatusCode: 404},
		{URL: base + "/b", Kind: crawler.ErrKindHTTPStatus, StatusCode: 500},
	}
	first := FindBrokenLinks(pages, errs)
	second := FindBrokenLinks(pages, errs)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic output: first=%#v second=%#v", first, second)
	}
}

const siteWideFixtureHTML = `<!DOCTYPE html><html><head><title>%s</title></head><body>%s</body></html>`

func siteWideLinkPage(title string, links ...string) string {
	var b strings.Builder
	for _, href := range links {
		fmt.Fprintf(&b, `<a href="%s">link</a>`, href)
	}
	return fmt.Sprintf(siteWideFixtureHTML, title, b.String())
}

func newSiteWideFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("User-agent: seo-audit\nDisallow: /private\n"))
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(siteWideLinkPage("Shared Title",
				"/dup-a", "/missing", "/private", "/badjson")))
		case "/dup-a":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(siteWideLinkPage("Shared Title")))
		case "/private":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(siteWideLinkPage("private page")))
		case "/badjson":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"not":"html"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func crawlSiteWideFixture(t *testing.T, srv *httptest.Server) crawler.CrawlResult {
	t.Helper()
	cfg := crawler.DefaultConfig()
	cfg.Retry = fastRetryForChecks()
	fetcher, err := crawler.NewFetcher(crawler.NewClient(), cfg)
	if err != nil {
		t.Fatalf("NewFetcher: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	result, err := crawler.Crawl(ctx, fetcher, srv.URL+"/", crawler.CrawlOptions{
		MaxPages: 100,
		MaxDepth: 5,
		Delay:    0,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	return result
}

func fastRetryForChecks() crawler.RetryConfig {
	return crawler.RetryConfig{
		MaxRetries:        0,
		BaseTimeout:       2 * time.Second,
		TimeoutMultiplier: 1.5,
		BaseBackoff:       5 * time.Millisecond,
		MaxTotalPerPage:   10 * time.Second,
	}
}

func TestSiteWideIntegration(t *testing.T) {
	srv := newSiteWideFixtureServer(t)
	result := crawlSiteWideFixture(t, srv)

	dups := FindDuplicateTitles(result.Pages)
	if len(dups) != 1 {
		t.Fatalf("duplicate titles: got %d groups, want 1: %#v", len(dups), dups)
	}
	urls, ok := dups["Shared Title"]
	if !ok || len(urls) != 2 {
		t.Fatalf("Shared Title group = %#v, want 2 URLs", urls)
	}

	broken := FindBrokenLinks(result.Pages, result.Errors)

	var missing404, parseFail *BrokenLink
	for i := range broken {
		bl := &broken[i]
		switch {
		case bl.TargetURL == srv.URL+"/missing":
			missing404 = bl
		case strings.HasSuffix(bl.TargetURL, "/badjson"):
			parseFail = bl
		case strings.HasSuffix(bl.TargetURL, "/private"):
			t.Errorf("robots-disallowed /private should not be flagged, got %#v", bl)
		}
	}

	if missing404 == nil {
		t.Fatal("expected broken link to /missing")
	}
	if missing404.Kind != crawler.ErrKindHTTPStatus || missing404.StatusCode != 404 {
		t.Errorf("/missing link: Kind=%v StatusCode=%d, want http_status/404", missing404.Kind, missing404.StatusCode)
	}

	if parseFail == nil {
		t.Fatal("expected broken link to /badjson")
	}
	if parseFail.Kind != crawler.ErrKindParseFailure {
		t.Errorf("/badjson link: Kind=%v, want parse_failure", parseFail.Kind)
	}
	if !strings.Contains(parseFail.Message, "application/json") {
		t.Errorf("/badjson Message = %q, want content-type detail", parseFail.Message)
	}
}

func TestBrokenLinkKindJSONRoundTrip(t *testing.T) {
	link := BrokenLink{
		SourceURL:  "https://example.com/",
		TargetURL:  "https://example.com/missing",
		Kind:       crawler.ErrKindHTTPStatus,
		StatusCode: 404,
		Message:    "not found",
	}

	var buf bytes.Buffer
	if err := json.MarshalWrite(&buf, link); err != nil {
		t.Fatalf("MarshalWrite: %v", err)
	}
	if !strings.Contains(buf.String(), `"kind":"http_status"`) && !strings.Contains(buf.String(), `"kind": "http_status"`) {
		t.Fatalf("JSON missing string kind: %s", buf.Bytes())
	}

	var decoded BrokenLink
	if err := json.UnmarshalRead(bytes.NewReader(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("UnmarshalRead: %v", err)
	}
	if decoded != link {
		t.Errorf("decoded = %#v, want %#v", decoded, link)
	}
}

func TestBrokenLinkKindJSONUnknown(t *testing.T) {
	const raw = `{"source_url":"https://example.com/","target_url":"https://example.com/x","kind":"bogus"}`
	var link BrokenLink
	err := json.UnmarshalRead(bytes.NewReader([]byte(raw)), &link)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !errors.Is(err, crawler.ErrUnknownErrorKind) {
		t.Fatalf("error = %v, want ErrUnknownErrorKind", err)
	}
}
