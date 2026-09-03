# seo-audit ΓÇö Staged Development Plan

Each stage is designed to be independently compilable and testable ΓÇö you should
be able to `go build && go test ./...` at the end of every stage. Stages are
ordered so each one only depends on what came before it, matching the
architecture diagram in the project doc (Crawler ΓåÆ Parser ΓåÆ Checks ΓåÆ Scoring ΓåÆ
Report ΓåÆ Regression ΓåÆ CI).

---

## Stage 0 ΓÇö Environment & Repo Scaffolding

**Goal:** A buildable empty Go module with the target directory layout and
dependency set pinned.

**Knowledge needed:**
- Go modules (`go mod init`, `go.mod`/`go.sum`, semantic import versioning)
- Go workspace conventions (`internal/` restricts imports to your own module)
- Cobra's generator conventions (`cmd/root.go` pattern)

**Work:**
```bash
mkdir seo-audit && cd seo-audit
go mod init github.com/<you>/seo-audit
go get github.com/spf13/cobra@latest
go get github.com/fatih/color@latest
go get golang.org/x/net/html@latest
# Decide GoQuery vs raw x/net/html now (see Stage 2 notes) ΓÇö pick one before writing the parser.
go get github.com/PuerkitoBio/goquery@latest   # if you choose GoQuery
```

Create the skeleton:
```text
seo-audit/
Γö£ΓöÇΓöÇ cmd/
Γöé   Γö£ΓöÇΓöÇ root.go
Γöé   Γö£ΓöÇΓöÇ crawl.go
Γöé   ΓööΓöÇΓöÇ compare.go
Γö£ΓöÇΓöÇ internal/
Γöé   Γö£ΓöÇΓöÇ crawler/
Γöé   Γö£ΓöÇΓöÇ parser/
Γöé   Γö£ΓöÇΓöÇ checks/
Γöé   Γö£ΓöÇΓöÇ scoring/
Γöé   Γö£ΓöÇΓöÇ report/
Γöé   ΓööΓöÇΓöÇ regression/
Γö£ΓöÇΓöÇ testdata/
Γö£ΓöÇΓöÇ main.go
Γö£ΓöÇΓöÇ go.mod
ΓööΓöÇΓöÇ README.md
```

`main.go`:
```go
package main

import "github.com/<you>/seo-audit/cmd"

func main() {
	cmd.Execute()
}
```

`cmd/root.go`:
```go
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "seo-audit",
	Short: "Crawl a site and evaluate SEO/technical-quality checks",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(2) // system/CLI error, per exit-code contract
	}
}
```

**Exit criteria:** `go build ./...` succeeds; `seo-audit --help` prints usage
with no subcommands yet.

---

## Stage 1 ΓÇö CLI Foundation (flags, no logic)

**Goal:** `crawl` and `compare` subcommands exist and parse all flags into a
typed config struct, but do nothing real yet (stub output only). This locks
the CLI contract early so every later stage just fills in behavior.

**Knowledge needed:**
- Cobra flag binding (`PersistentFlags` vs `Flags`, `StringVar`/`IntVar`)
- Designing a config struct that's passed down instead of reading globals
  everywhere (keeps `internal/` packages testable without Cobra)

**Work:**

`internal/config/config.go`:
```go
package config

type CrawlConfig struct {
	URL         string
	MaxPages    int
	MaxDepth    int
	Delay       time.Duration
	FailBelow   int
	Output      string // "text" | "json"
	Baseline    string // path to previous report, empty = none
	MaxScoreDrop int
}
```

`cmd/crawl.go`:
```go
var crawlCfg config.CrawlConfig

var crawlCmd = &cobra.Command{
	Use:   "crawl",
	Short: "Crawl a site and produce an SEO audit report",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Stage 1: just validate + echo config. Real logic added in later stages.
		fmt.Printf("%+v\n", crawlCfg)
		return nil
	},
}

func init() {
	crawlCmd.Flags().StringVar(&crawlCfg.URL, "url", "", "target URL (required)")
	crawlCmd.Flags().IntVar(&crawlCfg.MaxPages, "max-pages", 100, "max pages to crawl")
	crawlCmd.Flags().IntVar(&crawlCfg.MaxDepth, "max-depth", 5, "max crawl depth")
	crawlCmd.Flags().DurationVar(&crawlCfg.Delay, "delay", 200*time.Millisecond, "delay between requests")
	crawlCmd.Flags().IntVar(&crawlCfg.FailBelow, "fail-below", 0, "exit 1 if score below this")
	crawlCmd.Flags().StringVar(&crawlCfg.Output, "output", "text", "text|json")
	crawlCmd.Flags().StringVar(&crawlCfg.Baseline, "baseline", "", "path to baseline JSON report")
	crawlCmd.Flags().IntVar(&crawlCfg.MaxScoreDrop, "max-score-drop", 5, "regression threshold")
	crawlCmd.MarkFlagRequired("url")
	rootCmd.AddCommand(crawlCmd)
}
```

Same pattern for `cmd/compare.go` (`--baseline`, `--current`).

**Exit criteria:** `seo-audit crawl --url https://example.com --max-pages 10`
prints the parsed config struct. Missing `--url` produces a clean Cobra error,
not a panic.

---

## Stage 2 ΓÇö Single-Page Fetch + Parse

**Goal:** Given one URL, fetch it and extract a `PageData` struct. No
crawling/frontier yet ΓÇö this stage proves the fetchΓåÆparse pipeline in
isolation, which is the easiest place to catch parsing bugs.

**Knowledge needed:**
- `net/http`: custom `http.Client` with `Timeout`, checking `resp.StatusCode`,
  reading `Content-Type` header before parsing (skip non-HTML)
- HTML parsing approach ΓÇö decide now:
  - `golang.org/x/net/html`: lower-level tokenizer/tree, no query API, more
    code but zero extra dependency risk.
  - GoQuery: jQuery-style `.Find("h1")`, much faster to write checks against.
  - **Recommendation:** GoQuery for check-writing speed; the project doc lists
    it as an accepted option.
- Malformed HTML handling ΓÇö Go's HTML parsers are lenient (auto-close tags
  like a browser would), which matters for the "malformed HTML" parser test
  fixture.

**Work:**

`internal/crawler/fetch.go`:
```go
package crawler

type PageResponse struct {
	URL         string
	StatusCode  int
	ContentType string
	Body        []byte
	FetchErr    error
	Duration    time.Duration
}

func Fetch(client *http.Client, url string) PageResponse { ... }
```
Set a real `User-Agent` (e.g. `seo-audit/0.1 (+github.com/you/seo-audit)`) ΓÇö
some sites block empty/default Go UAs.

`internal/parser/parser.go`:
```go
package parser

type PageData struct {
	URL             string
	Title           string
	MetaDescription string
	H1Count         int
	H1Text          []string
	Images          []Image
	Canonical       string
	Viewport        string
	Links           []Link   // href + whether internal
	WordCount       int
}

type Image struct {
	Src, Alt string
}

type Link struct {
	Href     string
	Internal bool
	Text     string
}

func Parse(baseURL string, body []byte) (PageData, error) { ... }
```

Internal-vs-external link classification belongs here (compare parsed link
host against `baseURL` host) since it's needed by both the crawler frontier
(Stage 6) and the broken-link check (Stage 7).

**Parser test fixtures (`testdata/`)** ΓÇö build these now, they're reused
through Stage 4:
- `valid_page.html`
- `missing_title.html`
- `multiple_h1.html`
- `missing_alt.html`
- `malformed.html` (unclosed tags)
- `missing_canonical.html`
- `no_viewport.html`

Table-driven test pattern:
```go
func TestParse(t *testing.T) {
	cases := []struct {
		name     string
		file     string
		wantH1   int
		wantTitle string
	}{
		{"valid page", "valid_page.html", 1, "Example Title"},
		{"multiple h1", "multiple_h1.html", 2, "Example Title"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, _ := os.ReadFile(filepath.Join("../../testdata", c.file))
			pd, err := Parse("https://example.com", body)
			require.NoError(t, err)
			assert.Equal(t, c.wantH1, pd.H1Count)
		})
	}
}
```

**Exit criteria:** `go test ./internal/parser/...` passes on all seven
fixtures; a manual `Fetch` + `Parse` against a real URL (wired temporarily
into `crawl.go`) prints a populated `PageData`.

---

## Stage 3 ΓÇö URL Normalization & Dedup

**Goal:** A standalone, heavily-tested URL utility package. This is
deceptively fiddly and worth isolating before the crawler depends on it,
since bugs here silently cause double-crawls or missed pages.

**Knowledge needed:**
- `net/url` package (`url.Parse`, `ResolveReference` for relativeΓåÆabsolute)
- Normalization rules to pick and document: strip fragments (`#section`),
  decide on trailing-slash equivalence, decide whether to sort/strip tracking
  query params (or leave query strings alone for v1 ΓÇö simplest choice, note
  it in README), lowercase scheme/host.

**Work:**

`internal/crawler/normalize.go`:
```go
package crawler

func ResolveURL(base, ref string) (string, error) { ... } // relative -> absolute
func Normalize(rawURL string) (string, error) { ... }     // canonical form for the visited-set key
func SameDomain(a, b string) bool { ... }
```

Table-driven tests covering: relative path, relative with `../`,
protocol-relative (`//example.com/x`), fragment stripping, trailing-slash
equivalence, query-string pages treated as distinct URLs, `www.` vs bare
domain (decide once, document the decision ΓÇö don't silently merge them).

**Exit criteria:** Normalization test suite passes with edge cases from real
sites (test against a couple of live HTML samples with relative links).

---

## Stage 4 ΓÇö Check Framework + First Checks

**Goal:** The `Check` interface and enough checks to validate the framework
shape, run against `PageData` fixtures from Stage 2 ΓÇö no crawler needed yet.

**Knowledge needed:**
- Go interfaces and a simple registry pattern (slice of `Check`, not
  reflection-based auto-discovery ΓÇö keep it explicit for v1)
- Severity/deduction as data, not hardcoded per-check logic, so the scoring
  engine (Stage 5) can consume it uniformly

**Work:**

`internal/checks/check.go`:
```go
package checks

type Severity string

const (
	Pass    Severity = "pass"
	Info    Severity = "info"
	Warning Severity = "warning"
	Error   Severity = "error"
)

type CheckResult struct {
	CheckID   string
	Severity  Severity
	Message   string
	Deduction int
}

type Check interface {
	ID() string
	Name() string
	Run(parser.PageData) CheckResult
}

// Registry ΓÇö explicit list, not magic auto-registration.
func AllChecks() []Check {
	return []Check{
		TitleExistsCheck{},
		TitleLengthCheck{},
		MetaDescriptionCheck{},
		SingleH1Check{},
		ImageAltCheck{},
		CanonicalCheck{},
		ViewportCheck{},
	}
}
```

Single-page checks (site-wide ones ΓÇö duplicate titles, broken links ΓÇö wait
for Stage 7 since they need the full crawl result, not one page):

`internal/checks/title.go`:
```go
type TitleExistsCheck struct{}
func (TitleExistsCheck) ID() string   { return "TITLE_EXISTS" }
func (TitleExistsCheck) Name() string { return "Title tag exists" }
func (c TitleExistsCheck) Run(p parser.PageData) CheckResult {
	if strings.TrimSpace(p.Title) == "" {
		return CheckResult{c.ID(), Error, "Missing <title> tag", 10}
	}
	return CheckResult{c.ID(), Pass, "", 0}
}

type TitleLengthCheck struct{}
// warn if <30 or >60 chars, deduction 5
```

Same shape for `MetaDescriptionCheck`, `SingleH1Check` (0 H1s = error, 2+ =
warning), `ImageAltCheck` (percentage-based: deduct proportional to missing
alt coverage, or flat deduction above a missing-count threshold ΓÇö pick one
and document it), `CanonicalCheck`, `ViewportCheck`.

**Testing:** table-driven, feeding the Stage 2 `PageData` fixtures directly
into each check (no HTTP, no crawler ΓÇö this is the fastest test loop in the
whole project, lean on it).

**Exit criteria:** `go test ./internal/checks/...` green for all 7 checks
against all fixtures; e.g. `multiple_h1.html` ΓåÆ `SingleH1Check` returns
`Warning`.

---

## Stage 5 ΓÇö Scoring Engine

**Goal:** Deterministic page score and site score from a set of
`CheckResult`s, with configurable weighting.

**Knowledge needed:**
- Keep this stateless and pure (`func Score(results []CheckResult) int`) ΓÇö
  determinism is a stated non-functional requirement, so no time-based or
  map-iteration-order-dependent logic here (Go map iteration order is
  randomized, a classic source of nondeterminism bugs ΓÇö use slices, not
  map ranges, anywhere score math happens).

**Work:**

`internal/scoring/scoring.go`:
```go
package scoring

type Weights struct {
	Error   int // default -10
	Warning int // default -5
	Info    int // default 0
}

func DefaultWeights() Weights { return Weights{Error: 10, Warning: 5, Info: 0} }

func ScorePage(results []checks.CheckResult, w Weights) int {
	score := 100
	for _, r := range results {
		score -= r.Deduction
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

func ScoreSite(pageScores []int) int {
	if len(pageScores) == 0 {
		return 0
	}
	sum := 0
	for _, s := range pageScores {
		sum += s
	}
	return sum / len(pageScores) // integer average ΓÇö document rounding behavior
}
```

Note: deductions already live on `CheckResult` (Stage 4), so `Weights` here
is a hook for *future* configurability (e.g. loading weight overrides from a
config file) ΓÇö for v1 you can literally sum `r.Deduction` as shown. Keep the
`Weights` struct even if unused in v1 math, since "configurable weighting"
is an explicit requirement and it establishes where that would plug in.

**Testing:** perfect score (no failing checks ΓåÆ 100), multiple failures
(sum correctly), clamp-to-zero (many errors ΓåÆ 0, not negative), determinism
(run `ScorePage` twice on the same input, assert equal ΓÇö trivial but worth
having as a regression guard given the stated requirement).

**Exit criteria:** `go test ./internal/scoring/...` green.

---

## Stage 6 ΓÇö Crawl Engine (Frontier, robots.txt, Rate Limiting)

**Goal:** The real multi-page crawler: BFS frontier, robots.txt compliance,
depth/page limits, rate limiting, error classification. This is the
highest-complexity stage ΓÇö build it after Stages 2ΓÇô3 are solid since it
composes them directly.

**Knowledge needed:**
- BFS with a queue (slice-as-queue or `container/list`) + a `map[string]bool`
  visited set keyed on the Stage 3 normalized URL
- `robots.txt` parsing ΓÇö use an existing Go library (e.g.
  `github.com/temoto/robotstxt`) rather than hand-rolling the spec's
  edge cases (wildcard patterns, `Crawl-delay`, multiple user-agent blocks)
- Concurrency decision for v1: **recommend a single-goroutine sequential
  crawl** gated by `time.Sleep(delay)` between requests ΓÇö simpler, fully
  deterministic ordering (helps the "deterministic scoring" requirement and
  makes debugging tractable), and crawl speed isn't a stated bottleneck for
  v1. Note in README that a worker-pool version is a natural v2 upgrade if
  crawl time becomes a problem on larger sites.
- Distinguishing transient vs terminal errors: timeout/connection-refused are
  retryable-in-theory but for v1, log and skip (don't retry ΓÇö keeps runtime
  bounded and predictable for CI).

**Work:**

`internal/crawler/robots.go`:
```go
package crawler

type RobotsPolicy struct {
	group *robotstxt.Group
}

func FetchRobots(client *http.Client, siteURL string) (*RobotsPolicy, error) {
	// GET /robots.txt; on fetch failure, "fail safely" per spec ΓÇö for v1,
	// fail safe = allow all (log a warning), since blocking the whole crawl
	// because robots.txt is unreachable is worse for a QA tool than the
	// (documented) risk of crawling a page robots.txt would have disallowed.
}

func (p *RobotsPolicy) Allowed(path string) bool { ... }
```

`internal/crawler/frontier.go`:
```go
package crawler

type CrawlResult struct {
	Pages  []CrawledPage
	Errors []CrawlError
}

type CrawledPage struct {
	URL       string
	Depth     int
	Data      parser.PageData
	FetchTime time.Duration
}

type CrawlError struct {
	URL    string
	Reason string // "404", "403", "429", "500", "timeout", "connection", "invalid_html"
}

type CrawlOptions struct {
	MaxPages int
	MaxDepth int
	Delay    time.Duration
}

func Crawl(client *http.Client, robots *RobotsPolicy, startURL string, opts CrawlOptions) CrawlResult {
	type queueItem struct {
		url   string
		depth int
	}
	queue := []queueItem{{startURL, 0}}
	visited := map[string]bool{}
	result := CrawlResult{}

	for len(queue) > 0 && len(result.Pages) < opts.MaxPages {
		item := queue[0]
		queue = queue[1:]

		norm, err := Normalize(item.url)
		if err != nil || visited[norm] {
			continue
		}
		visited[norm] = true

		if !robots.Allowed(item.url) {
			continue
		}

		resp := Fetch(client, item.url)
		classifyAndMaybeRecord(&result, resp, item.url)
		if resp.FetchErr != nil || resp.StatusCode >= 400 {
			continue
		}

		pd, err := parser.Parse(item.url, resp.Body)
		if err != nil {
			result.Errors = append(result.Errors, CrawlError{item.url, "invalid_html"})
			continue
		}
		result.Pages = append(result.Pages, CrawledPage{item.url, item.depth, pd, resp.Duration})

		if item.depth < opts.MaxDepth {
			for _, link := range pd.Links {
				if link.Internal {
					queue = append(queue, queueItem{link.Href, item.depth + 1})
				}
			}
		}

		time.Sleep(opts.Delay)
	}
	return result
}
```

**Testing:** this is the one place `httptest.Server` earns its keep ΓÇö spin up
a local test server serving a small fixture site (3ΓÇô4 linked pages, one
robots.txt-disallowed page, one broken link, one 500) and assert the crawler
visits exactly the right set, respects `max-pages`/`max-depth`, and
classifies errors correctly. This is more valuable than mocking `Fetch`
directly because it exercises the real HTTP + robots.txt path together.

**Exit criteria:** crawler test suite passes against the local `httptest`
fixture server; manual run against a real small site completes without
hanging and respects `--delay`.

---

## Stage 7 ΓÇö Site-Wide Analysis

**Goal:** Cross-page checks that need the full `CrawlResult`, not a single
`PageData`: duplicate titles, broken internal links, and the aggregation
summary.

**Knowledge needed:**
- This doesn't fit the per-page `Check` interface from Stage 4 (it needs the
  whole site, not one page) ΓÇö model it as a separate `SiteCheck` step that
  runs after the crawl, or extend `checks.Check` with a second interface;
  keep it decoupled from `checks.Check` to avoid distorting that interface's
  single-page contract.

**Work:**

`internal/checks/sitewide.go`:
```go
package checks

type SiteResult struct {
	DuplicateTitles map[string][]string // title -> URLs sharing it
	BrokenLinks     []BrokenLink
}

type BrokenLink struct {
	SourceURL string
	TargetURL string
	Reason    string // reuses crawler.CrawlError reasons where applicable
}

func FindDuplicateTitles(pages []crawler.CrawledPage) map[string][]string { ... }
func FindBrokenLinks(pages []crawler.CrawledPage, crawlErrors []crawler.CrawlError) []BrokenLink { ... }
```

`internal/report/summary.go`:
```go
type Summary struct {
	PagesCrawled   int
	PagesWithErrors int
	BrokenLinks    int
	DuplicateTitles int
	AverageScore   int
}
```

**Exit criteria:** against the Stage 6 fixture server (which should include a
deliberate duplicate-title page and a broken internal link), the site-wide
checks correctly flag both.

---

## Stage 8 ΓÇö Reporting (JSON + Text)

**Goal:** Lock the JSON schema (the CI runner depends on this ΓÇö treat it as
an API contract, version it if you expect to change it later) and produce
matching human-readable terminal output.

**Knowledge needed:**
- `encoding/json` struct tags, `json.MarshalIndent` for readable output
- Keep stdout for the report and route diagnostic/progress logs to stderr
  (or a `--verbose` gated logger) ΓÇö this is explicitly required so CI can
  parse JSON stdout cleanly
- `fatih/color` for terminal formatting (auto-disables color when stdout
  isn't a TTY ΓÇö verify this behavior, don't assume)

**Work:**

`internal/report/report.go`:
```go
package report

type Report struct {
	URL          string          `json:"url"`
	Timestamp    time.Time       `json:"timestamp"`
	Score        int             `json:"score"`
	PagesCrawled int             `json:"pages_crawled"`
	Checks       []CheckSummary  `json:"checks"`
	Pages        []PageReport    `json:"pages"`
	Summary      Summary         `json:"summary"`
}

type PageReport struct {
	URL     string               `json:"url"`
	Score   int                  `json:"score"`
	Results []checks.CheckResult `json:"results"`
}

func BuildReport(url string, crawlResult crawler.CrawlResult, siteResult checks.SiteResult) Report { ... }

func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func (r Report) WriteText(w io.Writer) error {
	// fatih/color formatted: Γ£ô / Γ£ù / ΓÜá per check, score line, FAILED CHECKS section
}
```

**Testing:** golden-file test ΓÇö marshal a fixed `Report` struct, compare
against a checked-in `testdata/expected_report.json`; catches accidental
schema drift, which matters a lot once the CI runner is parsing this.

**Exit criteria:** `seo-audit crawl --url ... --output json` produces schema-
valid JSON on stdout with zero non-JSON bytes mixed in; `--output text`
produces the formatted terminal report from the project doc's mockup.

---

## Stage 9 ΓÇö Exit Codes / CI Gate Behavior

**Goal:** Wire the score into process exit codes per the documented contract.

**Knowledge needed:**
- `os.Exit` only at the outermost layer (`main.go`/`cmd/`) ΓÇö never inside
  `internal/` packages, or you make them untestable
- Cobra's `RunE` returning an error vs needing a distinct exit-code path:
  Cobra doesn't natively support custom exit codes per error type, so handle
  this explicitly in `cmd/crawl.go` rather than relying on `Execute()`'s
  generic error handling from Stage 0.

**Work:**

`cmd/crawl.go` (replacing the Stage 1 stub):
```go
RunE: func(cmd *cobra.Command, args []string) error {
	result, err := runCrawl(crawlCfg) // wraps Stages 6-8
	if err != nil {
		fmt.Fprintln(os.Stderr, "crawl error:", err)
		os.Exit(2)
	}

	// write report per --output
	...

	if crawlCfg.Baseline != "" {
		// Stage 10 regression check happens here
	}

	if result.Report.Score < crawlCfg.FailBelow {
		os.Exit(1)
	}
	return nil
},
```

**Exit criteria:** manual tests confirm `0`/`1`/`2` in each scenario ΓÇö
passing score, failing score, and (e.g.) an unreachable `--url`.

---

## Stage 10 ΓÇö Regression Engine

**Goal:** Compare current report against a baseline JSON file; detect score
drops and check-level PASSΓåÆFAIL flips; distinguish new failures from
pre-existing ones.

**Knowledge needed:**
- Structural diffing between two `Report` structs keyed by URL, then by
  `CheckID` within each page
- This is pure logic over two already-loaded `Report` values ΓÇö no I/O inside
  the diff function itself, which makes it trivial to unit test with two
  hand-built fixtures

**Work:**

`internal/regression/regression.go`:
```go
package regression

type RegressionResult struct {
	PreviousScore int
	CurrentScore  int
	Delta         int
	NewFailures   []Failure // present now, PASS/absent in baseline
	Resolved      []Failure // was failing in baseline, now passing
	Regressed     bool      // Delta < -maxScoreDrop OR len(NewFailures) > 0
}

type Failure struct {
	URL     string
	CheckID string
	Message string
}

func Compare(baseline, current report.Report, maxScoreDrop int) RegressionResult { ... }
```

`internal/regression/baseline.go`:
```go
func LoadBaseline(path string) (report.Report, error) { ... } // returns zero value, nil if path == "" or file missing (first run)
func SaveBaseline(path string, r report.Report) error { ... }
```

Wire into `cmd/crawl.go`: load baseline before crawling, compare after, print
the "SEO REGRESSION DETECTED" block from the doc's mockup on regression, then
`SaveBaseline` unconditionally at the end (today's report becomes tomorrow's
baseline regardless of pass/fail ΓÇö this is what makes the file-based
baseline scheme in the doc's "Baseline storage decision" section work without
any database).

**Testing:** table-driven cases: no baseline (first run, no regression
possible), improved score, degraded score under threshold, degraded score
over threshold, same score but a new check failure (should still flag),
check that flips FAILΓåÆPASS while another flips PASSΓåÆFAIL simultaneously.

**Exit criteria:** `seo-audit compare --baseline old.json --current new.json`
and the inline `crawl --baseline ...` path both produce correct
`RegressionResult`s against hand-built fixture JSON files.

---

## Stage 11 ΓÇö CI Runner Integration

**Goal:** Package the binary and wire it into the existing Go CI runner using
the file-based baseline scheme (no runner schema changes for v1).

**Knowledge needed:**
- Go cross-compilation (`GOOS`/`GOARCH`) for producing release binaries if
  the runner's environment differs from your dev machine
- Whatever the CI runner's job-definition format actually is (YAML shown in
  the doc) and how it exposes a **persistent per-job working directory** ΓÇö
  this is a prerequisite feature on the runner side, called out explicitly
  as "worth adding to the runner regardless of this project." If it doesn't
  exist yet, it's a small blocking task on the *other* project before this
  stage can be exercised for real.
- How the runner captures stdout/stderr and exit codes today, so you match
  its existing log-pipeline conventions rather than inventing a new one

**Work:**
1. Confirm/implement the runner's persistent job-dir feature (separate
   project, but a hard dependency for this stage).
2. `seo-audit crawl` reads `<job-dir>/latest.json` as `--baseline`
   automatically when the runner sets an env var or flag pointing at the job
   dir (decide: explicit `--baseline` flag passed by the job config, as
   shown in the doc's YAML, is simplest ΓÇö keep the tool itself unaware of
   "job dirs" as a concept, per the stated goal of zero coupling).
3. Add a `Makefile`/`goreleaser` config to produce a versioned release binary
   the runner fetches instead of building from source per run.
4. Add the job definition (from the doc) to the QRET/Degree Planner CI
   config.

**Exit criteria:** a real CI run executes `seo-audit crawl`, exit code
propagates correctly to the runner's pass/fail logic, and `latest.json`
persists between two consecutive runs (verify the second run's regression
comparison actually fires against the first run's report).

---

## Stage 12 ΓÇö Production Validation & Docs

**Goal:** Prove it on real sites, catch false positives, and document
everything so someone other than you can operate it.

**Knowledge needed:** mostly judgment calls, not new technical knowledge ΓÇö
this is where check thresholds (title length, alt-text deduction curve) get
tuned against real-world pages that legitimately don't match the "textbook"
SEO structure your fixtures assumed.

**Work:**
- Run against QRET and Degree Planner; log every check firing and manually
  eyeball for false positives (e.g. a legitimately short title, a
  decorative image intentionally missing alt text).
- Basic perf check: time + memory for a ~50ΓÇô100 page crawl; confirm it's
  nowhere near CI timeout limits.
- Failure-injection test: deliberately break a title, duplicate a title,
  remove a canonical, break a link, add a second H1 ΓÇö rerun, confirm each
  is caught and correctly reflected in exit code + regression output.
- Set up the nightly scheduled run (independent trigger from the CI runner,
  not tied to deploys) ΓÇö content/CMS changes can regress SEO without any
  code changing.
- Write `README.md`: install, CLI usage/flags, check catalog + how each
  scores, exit codes, full JSON schema (reference the Stage 8 golden file),
  CI job config, "how to add a new check" (walk through implementing
  `checks.Check` and adding it to `AllChecks()`), baseline/regression
  behavior.

**Exit criteria:** documented, tuned, running nightly, and integrated into
at least one real deploy pipeline without false-positive noise.

---

## Stage 13 ΓÇö Deferred / v2 (not built now, just noted)

Tracked for later, explicitly not blocking v1 completion:
- Full schema.org / JSON-LD validation
- JS-rendered page auditing (would require headless browser ΓÇö significant
  architecture change, e.g. chromedp)
- Sitemap intelligence
- Redirect-chain tracking
- Orphan-page analysis
- Duplicate meta-description detection
- `job_artifacts` SQLite table on the CI runner + score-history dashboard

No LLM-suggestion layer is planned at all for this tool.

---

## Suggested Build Order Summary

| Stage | Depends on | Can be tested in isolation? |
|---|---|---|
| 0. Scaffolding | ΓÇö | trivially |
| 1. CLI flags | 0 | yes (manual) |
| 2. Fetch + Parse | 0 | yes, fully unit-testable |
| 3. URL normalize | 0 | yes, fully unit-testable |
| 4. Checks | 2 | yes, fully unit-testable |
| 5. Scoring | 4 | yes, fully unit-testable |
| 6. Crawl engine | 2, 3 | yes, via httptest fixture server |
| 7. Site-wide analysis | 6 | yes, via httptest fixture server |
| 8. Reporting | 5, 7 | yes, golden-file test |
| 9. Exit codes | 8 | manual/integration |
| 10. Regression | 8 | yes, fully unit-testable |
| 11. CI integration | 9, 10 | integration only, needs runner-side prereq |
| 12. Validation & docs | 11 | manual |
| 13. v2 backlog | ΓÇö | n/a |

Stages 2ΓÇô5 and 10 are the ones you can build and fully unit-test with zero
network access ΓÇö front-load those if you want fast, isolated progress before
tackling the crawler's integration-test complexity in Stage 6.
