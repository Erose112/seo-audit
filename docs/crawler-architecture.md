# SEO Audit CLI — Crawler / HTTP / Timeout / Context Architecture

**Scope:** This document covers the design of the crawling subsystem only —
HTTP client construction, timeout strategy, context/cancellation hierarchy,
retry behavior, sequential crawl loop, and the boundary between page-level and
systemic failures. It assumes the surrounding CLI, checks engine, scoring,
and reporting layers described in the main development plan.

---



## 1. Design Goals

- Single target host, user-supplied URL, multi-page BFS crawl.
- Must behave predictably as a **CI quality gate**: hangs, partial failures,
and ambiguous states are worse than clean failures.
- Must not overload the target site (it's the user's own site, but often on
modest hosting).
- Failures must be classified correctly so the exit-code contract
(`0` = pass, `1` = gate failed, `2` = crawl/system error) stays meaningful.
- Deterministic, bounded resource use: a single bad page or unreachable host
must not stall or crash the whole audit.
- Deterministic page selection under `--max-pages`: for an unchanged site,
two consecutive runs must audit the same URL subset. The crawl order is a
pure function of link order within fetched pages — not of network latency.
This is required for the CI regression/baseline gate to be trustworthy.

---



## 2. Context Hierarchy

A single root context governs the entire crawl; every page fetch derives a
child context from it. This is the backbone the rest of the system hangs off.

```go
func NewCrawlContext(maxDuration time.Duration) (context.Context, context.CancelFunc) {
    ctx, cancel := context.WithTimeout(context.Background(), maxDuration)
    // SIGTERM matters as much as SIGINT here: GitHub Actions job cancellation,
    // Kubernetes pod termination, and `docker stop` all send SIGTERM, not
    // Ctrl+C. Without it, CI-initiated cancellation bypasses this cancellation
    // path entirely and falls back to the OS's default SIGTERM handling —
    // meaning none of the exit-code contract in §7/§8 gets honored for the
    // single most likely real-world cancellation source for this tool.
    ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
    return ctx, func() { stop(); cancel() }
}
```

**Decision: add** `--max-duration` **as a first-class CLI flag** (default e.g.
`5m`), used as the root context deadline.

*Rationale:* The existing flags (`--max-pages`, `--max-depth`, `--delay`)
bound pages, not wall-clock time. Without a time budget, a large or
misbehaving site has no natural ceiling, which is unacceptable for a CI gate
that other builds are waiting on.

### Two independent cancellation paths


| Trigger                                                                             | Effect                                                                                                     |
| ----------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Root deadline (`--max-duration`) elapses, or Ctrl+C/SIGTERM (incl. CI cancellation) | Cancels the in-flight fetch (at most one under sequential crawl) via context propagation — no manual bookkeeping required. |
| Per-page context deadline (including retries) elapses                               | Cancels **only that fetch**; the crawl loop catches it, records it as a page-level error, and continues.                   |


Every subsystem that can block — the HTTP fetch, the rate limiter's delay,
and the retry backoff — must select on `ctx.Done()` so that root
cancellation actually propagates instead of leaving the process to finish
whatever it was doing.

---



## 3. HTTP Client / Transport

Built once per crawl and shared across all requests (same host throughout,
so connection reuse matters).

```go
var ErrTooManyRedirects = errors.New("stopped after 5 redirects")

// NewClient takes no Config: transport timeouts/caps are fixed (below).
// User-Agent, MaxBodySize, and Retry live on Fetcher via crawler.Config.
func NewClient() *http.Client {
    transport := &http.Transport{
        DialContext: (&net.Dialer{
            Timeout:   5 * time.Second,
            KeepAlive: 30 * time.Second,
        }).DialContext,
        TLSHandshakeTimeout: 5 * time.Second,
        IdleConnTimeout:     90 * time.Second,
        // Sequential crawl: one in-flight request at a time. Cap the transport
        // to match so a future concurrency change can't silently open more
        // connections than the crawl loop intends.
        MaxIdleConnsPerHost: 1,
        MaxConnsPerHost:     1,
        // No ResponseHeaderTimeout — see below, same reasoning as Client.Timeout.
    }

    return &http.Client{
        Transport: transport,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            if len(via) >= 5 {
                return ErrTooManyRedirects // sentinel, not a bare errors.New — needed so
                                           // classifyFetchError (§6) can route this to a
                                           // non-retryable kind instead of the generic
                                           // connection-error bucket
            }
            return nil
        },
        // No Client.Timeout — per-page context deadlines supersede it.
    }
}
```



### Decision: transport phase timeouts are fixed and conservative, not user-tunable


| Timeout               | Value | Rationale                                                                                                                                                    |
| --------------------- | ----- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `Dialer.Timeout`      | 5s    | Only paid once or twice per crawl (connections are reused via keep-alive); a dead/unreachable host should fail fast regardless of page-level timeout policy. |
| `TLSHandshakeTimeout` | 5s    | Same reasoning — a hung handshake indicates a broken TLS endpoint or middlebox, not a slow page.                                                             |
| `IdleConnTimeout`     | 90s   | Standard keep-alive hygiene; not user-facing.                                                                                                                |


`Dialer.Timeout` and `TLSHandshakeTimeout` stay fixed because they're each
paid at most once per connection (kept alive and reused afterward), so they
don't interact with the per-attempt escalation in §5 — a hung dial or
handshake indicates a broken endpoint regardless of which retry attempt
triggered it.

### Decision: `Client.Timeout` **and** `ResponseHeaderTimeout` are deliberately **not set**

**Revision:** the original design kept `Client.Timeout` unset but left
`ResponseHeaderTimeout: 15s` on the transport. That's inconsistent —
`ResponseHeaderTimeout` is a *transport-level, non-per-request* setting, so
it silently caps every attempt at 15s regardless of the context deadline
passed in for that attempt. Since §5's retry design deliberately escalates
the per-attempt timeout on retries (15s → 22.5s → 33.75s) specifically to
give slow-to-respond CMS sites more room on a second try, a fixed
`ResponseHeaderTimeout: 15s` would silently defeat that on every retry —
attempts 2 and 3 would still die waiting on headers at the 15s mark no
matter what the context deadline said.

Per-page context deadlines are strictly more expressive than either
knob — they support parent-driven cancellation and per-attempt override —
so carrying `Client.Timeout` or a fixed `ResponseHeaderTimeout` alongside a
context deadline is redundant at best (for attempt 1) and silently
incorrect at worst (for retries). Both are left unset; the context deadline
passed into each `fetchOnce` call is the sole timeout mechanism for
dial-to-body-start latency on that attempt.

### Decision: redirect cap of 5 hops

Prevents a misconfigured redirect loop from silently consuming the page's
timeout budget across many hops before failing.

---



## 4. Crawl Loop Concurrency



### Decision: single-goroutine sequential crawl for v1 — no `--workers` flag

v1 fetches one page at a time from a BFS frontier. There is no worker pool
and no `--workers` CLI flag. Pacing is a context-aware delay between
requests (§9); transport connection caps are fixed at 1 to match (§3).

```go
// Public Stage 6 API (landed). Internally sequential BFS; not a *Crawler method.
func Crawl(ctx context.Context, fetcher *Fetcher, startURL string, opts CrawlOptions) (CrawlResult, error) {
    // 1. Normalize startURL → seed seen; fetch root (any failure → systemic).
    // 2. FetchRobots (fail-open); enqueue root links (robots + SameDomain + seen).
    // 3. Sequential BFS until queue empty or MaxPages reached:
    //      rateLimiter.Wait(ctx) → FetchWithRetry → record / enqueue
    //      incremental checkSystemicFailureRate once attempts >= 5
    // 4. ErrKindCanceled propagates immediately.
}
```

**Status (Stage 6 landed):** the public API is the free function above, not a
`*Crawler` method. Root is fetched before robots and before the BFS loop
(§12 / prerequisites). Robots gating, `Normalize` before `seen`, and
incremental `checkSystemicFailureRate` are wired. The important architectural
points:

- **Deterministic truncation under `--max-pages`:** queue order depends only
  on link discovery order within each page. With one fetch at a time, which
  pages land under the cap is a pure function of site content + flags —
  required for baseline/regression comparison (§1).
- **No quiescence/coordinator machinery:** sequential crawl has at most one
  in-flight fetch, so "frontier empty or cap reached" is sufficient to stop.
  There is no race between workers finishing and the cap tripping.
- **Request rate ≈ `1 / delay`:** document that for users; there is no
  `workers / delay` aggregate.

*Rationale:* Concurrent workers racing a shared `--max-pages` cap make the
audited URL set depend on response-latency jitter. Two runs against an
unchanged site can select different subsets and produce false regressions.
The sites large enough to want parallelism are exactly the sites that hit
the cap — concurrency helps most where truncation noise hurts most. Crawl
speed is not a stated v1 bottleneck; trustworthy CI gating is.

**Supersedes:** the earlier draft of this section (`--workers` default 4,
hard max 10, worker pool + channel coordinator, "stop enqueuing but let
in-flight finish"). That design is incompatible with deterministic
page-set selection under a hard page cap.

### Deferred: parallel fetches (v2+)

If crawl wall-clock becomes a real problem on large sites, revisit
concurrency only with a scheduler that preserves determinism — e.g.
**level-synchronous BFS** (fetch an entire depth level, possibly in
parallel; merge and stably order discoveries; then admit the next level /
apply `--max-pages` to that ordered frontier). A free-racing worker pool
against a shared cap must not return. Until then, do not expose a
`--workers` flag; shipping it invites the nondeterministic mode CI should
not use.

---



## 5. Retry Strategy



### Decision: retries are in scope for v1, with escalating per-attempt timeouts

The per-page timeout stays **fixed** (not adaptive per-URL — there's no
reliable way to know in advance which pages are legitimately slower), but
each **retry** of the same page gets a longer timeout, since a timeout may
indicate the page needed more time rather than being genuinely dead.

```go
type RetryConfig struct {
    MaxRetries        int           // default 2 (3 attempts total)
    BaseTimeout       time.Duration // default 15s
    TimeoutMultiplier float64       // default 1.5  → 15s → 22.5s → 33.75s
    BaseBackoff       time.Duration // default 1s, exponential
    MaxTotalPerPage   time.Duration // hard ceiling: 90s (was 60s — see rationale below)
}

func (f *Fetcher) FetchWithRetry(parent context.Context, url string, rc RetryConfig) (*PageResponse, error) {
    // Hard per-page ceiling, enforced independently of the escalation
    // schedule below — this WithTimeout call is what actually makes
    // MaxTotalPerPage a ceiling rather than just a documented intention.
    ctx, cancel := context.WithTimeout(parent, rc.MaxTotalPerPage)
    defer cancel()

    var lastErr error
    timeout := rc.BaseTimeout

    for attempt := 0; attempt <= rc.MaxRetries; attempt++ {
        if parent.Err() != nil {
            return nil, parent.Err() // root context dead — stop immediately, propagate up (systemic)
        }
        if ctx.Err() != nil {
            // per-page ceiling exhausted — a page-level timeout, not systemic;
            // return now instead of burning remaining attempts on a context
            // that will fail instantly anyway
            return nil, fmt.Errorf("per-page ceiling (%s) exceeded after %d attempt(s): %w", rc.MaxTotalPerPage, attempt, lastErr)
        }

        resp, err := f.fetchOnce(ctx, url, timeout) // ctx, not parent — inherits the ceiling
        if err == nil {
            return resp, nil
        }
        lastErr = err

        if !isRetryable(err) {
            return nil, err // fail fast — see classification below
        }

        if attempt < rc.MaxRetries {
            backoff := time.Duration(float64(rc.BaseBackoff) * math.Pow(2, float64(attempt)))
            select {
            case <-time.After(backoff):
            case <-ctx.Done(): // fires on root cancellation OR ceiling expiry, whichever is first
                return nil, ctx.Err()
            }
            timeout = time.Duration(float64(timeout) * rc.TimeoutMultiplier)
        }
    }
    return nil, fmt.Errorf("all retries exhausted: %w", lastErr)
}
```

Note that classification (§6) already does the right thing with the two
possible `ctx.Err()` values from the backoff `select`: if the *root*
context died, `ctx.Err()` is `context.Canceled` → `ErrKindCanceled` →
systemic propagation; if only the per-page *ceiling* expired, `ctx.Err()`
is `context.DeadlineExceeded` → `ErrKindTimeout` → an ordinary page-level
result. No extra branching is needed to distinguish the two cases.

### Decision: hard ceiling raised to ~90s total per page (was 60s)

**Revision:** the original 60s ceiling was inconsistent with its own
escalation schedule and, worse, was never actually enforced in code —
`MaxTotalPerPage` was declared on the struct but nothing in `FetchWithRetry`
referenced it. Fixed both problems together:

- **Enforcement:** `FetchWithRetry` now derives a ceiling-bound child
context via `context.WithTimeout(parent, rc.MaxTotalPerPage)` and uses it
for every attempt and for the backoff wait, so the ceiling is a real hard
stop rather than a struct field nobody reads.
- **Sizing:** the nominal 3-attempt schedule is `15s + 1s(backoff) + 22.5s + 2s(backoff) + 33.75s ≈ 74.25s`. A 60s ceiling would truncate attempt 3 by
up to ~42% before enforcement even kicks in, defeating the point of
computing an escalated timeout for it. 90s gives ~16s of margin above the
nominal worst case (for scheduling jitter, not a design assumption to
rely on) while still bounding any single dead page to at most 30% of the
default 300s (`--max-duration 5m`) budget.



### Decision: retry loop must itself respect the root context

Both the `parent.Err()` check at the top of each attempt (for immediate
systemic propagation) and the `select` on `ctx.Done()` during backoff (for
either root cancellation or ceiling expiry) are required — otherwise root
cancellation (timeout, Ctrl+C, or SIGTERM) could be delayed by up to a full
backoff/retry cycle before the crawl actually stops.

### Retryability classification

```go
func isRetryable(err error) bool {
    var fe *FetchError
    if !errors.As(err, &fe) {
        return false
    }
    switch fe.Kind {
    case ErrKindTimeout, ErrKindConnection:
        return true // transient network issues — worth another attempt
    case ErrKindCanceled:
        return false // root context dying — never retry, propagate immediately
    case ErrKindTLS, ErrKindPermanent:
        return false // deterministic failures (bad cert, NXDOMAIN, redirect loop) —
                      // retrying reproduces the identical outcome and just burns
                      // the page's timeout budget for nothing
    case ErrKindHTTPStatus:
        return fe.StatusCode == 429 || fe.StatusCode >= 500
        // 4xx other than 429 (404, 403, 401, 400) are NOT retryable —
        // these are legitimate findings, not transient glitches; retrying
        // them wastes time and cannot change the outcome.
    }
    return false
}
```

**Revision:** the original switch had no case for `ErrKindTLS` and let it
fall through to the trailing `return false` — correct by accident, but
`ErrKindTLS` was never actually *produced* by `classifyFetchError` either
(see §6), so it was dead code carrying an implicit promise it didn't keep.
The cases are now explicit, and §6 below is updated so DNS/TLS/redirect
failures actually get classified into a non-retryable kind instead of
silently falling into the retryable `ErrKindConnection` bucket.

---



## 6. Error Classification

A shared error type distinguishes cause, which drives both retry eligibility
(§5) and the systemic-vs-page-level exit code decision (§7).

```go
type FetchErrorKind int

const (
    ErrKindTimeout FetchErrorKind = iota
    ErrKindConnection
    ErrKindTLS
    ErrKindHTTPStatus
    ErrKindPermanent // deterministic, non-network failure (DNS NXDOMAIN, redirect loop) —
                      // retrying can't produce a different outcome
    ErrKindCanceled  // parent/root context died — always systemic
    // ErrKindParseFailure: HTTP succeeded but body is non-HTML, truncated, or
    // unparseable. Recorded on CrawlResult.Errors; excluded from §7 fail-rate.
    ErrKindParseFailure
)

type FetchError struct {
    Kind       FetchErrorKind
    StatusCode int
    Err        error
}

func classifyFetchError(err error) error {
    if errors.Is(err, context.Canceled) {
        return &FetchError{Kind: ErrKindCanceled, Err: err}
    }
    if errors.Is(err, context.DeadlineExceeded) {
        return &FetchError{Kind: ErrKindTimeout, Err: err}
    }
    if errors.Is(err, ErrTooManyRedirects) {
        return &FetchError{Kind: ErrKindPermanent, Err: err} // §3 — same loop every retry
    }

    var dnsErr *net.DNSError
    if errors.As(err, &dnsErr) {
        if dnsErr.IsTimeout {
            return &FetchError{Kind: ErrKindTimeout, Err: err} // resolver was slow — worth another try
        }
        return &FetchError{Kind: ErrKindPermanent, Err: err} // e.g. NXDOMAIN — won't resolve differently
    }

    var certErr x509.UnknownAuthorityError
    var certInvalidErr x509.CertificateInvalidError
    var hostnameErr x509.HostnameError
    if errors.As(err, &certErr) || errors.As(err, &certInvalidErr) || errors.As(err, &hostnameErr) {
        return &FetchError{Kind: ErrKindTLS, Err: err} // cert/hostname problems don't self-resolve on retry
    }

    var netErr net.Error
    if errors.As(err, &netErr) && netErr.Timeout() {
        return &FetchError{Kind: ErrKindTimeout, Err: err}
    }
    return &FetchError{Kind: ErrKindConnection, Err: err}
}
```

**Revision:** the original version only distinguished cancellation, timeout,
and a catch-all `ErrKindConnection` — meaning DNS `no such host`, TLS
certificate/hostname failures, and the §3 redirect-loop sentinel all fell
into the retryable bucket. These are all deterministic: a typo'd domain, an
expired certificate, and a redirect loop all reproduce identically on
retry, so retrying them only burns backoff time and per-attempt timeout
budget for a guaranteed-identical result. This matters most on **Rule 1**
(§7) — the root-URL-unreachable check — since that's exactly the path where
a bad hostname is most likely to appear, and it's the check the codebase
most wants to fail fast rather than spend ~74s retrying a typo.

---



## 7. Systemic vs. Page-Level Failure Boundary (Exit Code 2)



### Decision: draw the line at "can we trust the report we're about to produce," not "did anything fail"

```go
func (c *Crawler) evaluateSystemicFailure(results *CrawlResults) error {
    // Rule 1: root URL itself unreachable after retries — nothing to audit.
    if results.RootPageFailed {
        return fmt.Errorf("root URL unreachable: %w", results.RootError)
    }

    // Rule 2: robots.txt fetch failure is fail-open, not systemic
    // (unreachable robots.txt is treated as "no restrictions").

    // Rule 3: failure rate too high to trust the score.
    if results.PagesCrawled > 0 {
        total := results.PagesCrawled + results.PagesFailed
        failRate := float64(results.PagesFailed) / float64(total)
        if failRate > 0.5 && total >= 5 {
            return fmt.Errorf(
                "failure rate %.0f%% suggests the site is unreachable, not audit-worthy",
                failRate*100,
            )
        }
    }

    return nil // individual page failures are scored as check results (exit 0/1 territory)
}
```

**Rules:**

1. **Root URL unreachable after retries → exit 2, always.** This is checked
  as a special case before entering the general crawl loop — the root page
   isn't "just another page," it's the precondition for the whole audit.
2. `robots.txt` **unreachable → not systemic.** Consistent with the original
  plan's "fail safely if unreachable" — treated as no restrictions found.
3. **>50% of attempted pages failed, with at least 5 pages attempted → exit
  2.** A handful of broken links or timeouts is exactly what the tool
   exists to catch and should be scored normally (exit 0/1). But if roughly
   half or more of the site is failing, that's evidence the site itself is
   down, blocking the crawler, or misconfigured — reporting a score in that
   state would let a real regression hide behind "half the crawl failed
   anyway." The `≥5` guard avoids false-triggering on small sites where 1
   failure out of 2 pages looks like 50%.
4. `ErrKindCanceled` **(root context died) always propagates immediately**
  out of the crawl loop, regardless of how many pages had
   already succeeded — see §8.

Page-level failure reasons (timeout vs. 404 vs. 5xx, and whether retries
were attempted) should be logged per page so a human investigating an
exit-2 CI failure isn't left guessing why.

**Clarification: what counts toward** `PagesFailed`**.** Only pages that were
actually fetched and returned a non-nil **fetch** error count —
pages never reached because `--max-pages` or `--max-depth` cut the frontier
off first are neither crawled nor failed, and must not be added to either
side of the Rule 3 ratio (they'd dilute or inflate `failRate` for reasons
unrelated to reachability). Parse / non-HTML / truncated bodies
(`ErrKindParseFailure`) are also excluded: the HTTP layer succeeded; Rule 3
is about site reachability, not markup quality.

### Revision: check Rule 3 incrementally, not only at the end

As originally written, `evaluateSystemicFailure` only runs once, after the
crawl loop finishes — so a badly broken site burns through its *entire*
time and page budget (including up to 90s of retries per failing page,
post-§5-revision) before the tool reports exit 2. For a CI gate where other
builds are waiting, that's the one place slow failure is most costly. The
sequential crawl loop (§4) should apply the same `failRate > 0.5 && total >= 5`
check after each page once `total >= 5`, and stop dequeuing new work the
moment it trips — treating it the same as Rule 1 (fail fast) rather than
only the same as "check once at the end." The final `evaluateSystemicFailure`
call stays as a safety net for the case where the threshold is crossed by
the very last few pages, where an incremental check wouldn't have had a
chance to fire first.

---



## 8. Cancellation Behavior (Ctrl+C / Root Deadline)



### Decision: hard-exit on cancellation, no partial report (for now)

When the root context is cancelled — whether by `--max-duration` elapsing,
`SIGINT` (Ctrl+C), or `SIGTERM` (CI cancellation, see §2) — the crawl stops
and the process exits without emitting a JSON report.

**Clarification:** exit code 2 now covers several distinct situations
(deadline exceeded, SIGINT, SIGTERM, root-URL-unreachable, high failure
rate). The exit code itself should stay uniform — that's the point of the
exit-code contract — but stderr should say plainly which of these occurred
(e.g. `"crawl cancelled: --max-duration (5m) exceeded"` vs. `"crawl cancelled: received SIGTERM"`) so a human or CI log isn't left inferring
the cause from timing alone.

*Rationale:* A partial report risks being misread by the CI runner as a
complete regression check, which is worse than a clean, unambiguous failure.
This keeps the exit-code contract simple: cancellation is treated the same
as any other systemic failure (exit 2), rather than introducing a third
report state that the regression engine and dashboard would need to handle
specially. Revisit if local/interactive use later wants partial output for
debugging — that would be a separate, explicitly-flagged mode
(e.g. `--allow-partial-report`), not the default.

```go
for _, url := range frontier.Next() {
    resp, err := fetcher.FetchWithRetry(ctx, url, retryConfig)
    if err != nil {
        var fe *FetchError
        if errors.As(err, &fe) && fe.Kind == ErrKindCanceled {
            return err // propagate — Run() exits 2, no report written
        }
        results.RecordPageError(url, err) // page-level — continue crawling
        continue
    }
    results.RecordPage(resp)
}
```

---



## 9. Rate Limiter — Context-Aware by Necessity



### Decision: rate limiter delay must select on `ctx.Done()`, not use a bare `time.Sleep`

```go
func (r *RateLimiter) Wait(ctx context.Context) error {
    select {
    case <-time.After(r.delay):
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

*Rationale:* A bare `time.Sleep(r.delay)` is not interruptible. Without this,
root cancellation could be delayed by up to one full `--delay` period
before the process actually stops — undermining the hard-exit behavior
decided in §8.

**Naming clarification:** `RateLimiter` here is a fixed inter-request pacing
delay, not a shared token bucket. Under sequential crawl the effective rate
is approximately `1 / delay` (plus fetch latency). That's fine for this
tool's actual goal (avoid being egregious against the user's own modest
hosting, not hit an exact QPS target), but the docs/flag help text
shouldn't imply it's a precise cap.

---



## 10. robots.txt Fetch

Uses the same shared HTTP client, but with its own short, fixed timeout
(10s) derived from the root context — independent of the per-page retry
timeout model in §5, since it's fetched once at crawl start and a slow
`robots.txt` shouldn't consume page-level budget. Per §7 Rule 2, failure to
fetch it is treated as "no restrictions found," not a systemic error.

**Landed (Stage 6):** `FetchRobots(ctx, client, siteURL, userAgent)` in
`robots.go` using `github.com/temoto/robotstxt`. Always returns a non-nil
`*RobotsPolicy` (allow-all on failure). Called inside `Crawl` **after** the
root fetch. Does not use `FromStatusAndBytes` (its 5xx → disallow-all
conflicts with fail-open); non-2xx and network errors fail open.

**Decision:** `robots.txt`**'s** `Crawl-delay` **directive is parsed for**
`Disallow`**/**`Allow` **rules but not applied to pacing in v1.** Both it and
`--delay` serve the same "don't overload the target" goal, but honoring a
site's `Crawl-delay` automatically could silently make `--delay` a no-op or
produce a much slower crawl than the user configured, which is a confusing
interaction for a CI tool where predictable run time matters (§1). Worth
revisiting as an explicit `--respect-crawl-delay` flag later; out of scope
for v1.

---



## 11. Response Body Size Limit



### Decision: 2 MB cap, configurable via `--max-body-size`

```go
const cap = cfg.MaxBodySize // default 2 * 1024 * 1024

// Read one byte past the cap: io.LimitReader alone can't distinguish "body
// was exactly at the cap" from "body was truncated", since both produce
// exactly cap bytes of output.
data, err := io.ReadAll(io.LimitReader(resp.Body, cap+1))
if err != nil {
    return nil, err
}
truncated := int64(len(data)) > cap
if truncated {
    data = data[:cap]
}
```

*Rationale:* SEO checks only need `<head>` metadata, headings, links, and
image `alt` attributes — not inline assets. Typical HTML documents run tens
of KB to a few hundred KB; even heavy CMS output with significant inline
script/tracking noise generally stays under 1–1.5 MB. 2 MB gives headroom
above that without meaningfully increasing exposure to a misbehaving or
oversized response. Exposed as a flag rather than hardcoded, since some
poorly-optimized CMS output (which may itself be worth flagging) can exceed
it.

**Note:** Go's `http.Transport` transparently gzip-decodes response bodies
by default, so this cap applies to *decompressed* size. That's consistent
with the rationale above (it's decompressed HTML size the "tens of KB to a
few hundred KB" estimate is really about), but worth stating explicitly —
it's an easy thing for an implementer to assume is measured against wire
bytes instead.

**Handling truncation:** if the read hits the cap (the `cap+1`-byte read
above actually consumed that extra byte), the page sets
`PageResponse.Truncated` (landed). The crawl records
`ErrKindParseFailure` rather than feeding truncated HTML into the parser.
Stage 7's `FindBrokenLinks` flags pages that link to such targets (and other
`CrawlError`s); it is not a separate per-page `Check`.

---



## 12. End-to-End Flow Summary

**Status:** Stages 6–8 landed. `cmd/crawl.go` runs `runCrawl` (Crawl →
per-page checks/scoring → site-wide analysis → `report.BuildReport`) and
writes JSON or text. Exit-code mapping remains Stage 9.

```
cmd/crawl: NewCrawlContext(maxDuration) → runCrawl(ctx, cfg)
  │
  ├─ NewFetcher(NewClient(), cfg) → Crawl
  │    ├─ fetch root URL (special case: failure here → systemic err immediately)
  │    ├─ fetch + parse robots.txt (10s timeout, fail-open; root ignores robots)
  │    │
  │    └─ sequential BFS loop (one in-flight fetch; MaxConnsPerHost = 1)
  │         └─ per queued URL, until frontier empty or --max-pages reached:
  │              1. rateLimiter.Wait(ctx)              — ctx-aware, exits clean on cancel
  │              2. Fetcher.FetchWithRetry(ctx, url, retry):
  │                   attempt 1: fetchOnce(ctx, url, 15s)
  │                     ├─ success → done
  │                     └─ fail → isRetryable?
  │                          ├─ no  (404/403/canceled/TLS/permanent) → return immediately
  │                          └─ yes → backoff, escalate timeout ×1.5, retry
  │                                (max 2 retries, ≤90s total per page, enforced)
  │              3. classify result:
  │                   ├─ ErrKindCanceled → propagate → exit 2, no report (hard-exit)
  │                   ├─ other fetch error → append CrawlError; continue
  │                   ├─ parse/non-HTML/truncated → ErrKindParseFailure (not fail-rate)
  │                   └─ success → append CrawledPage; enqueue undiscovered internal links
  │                        (ResolveURL + SameDomain; robots on path+query; seen = Normalize)
  │              4. checkSystemicFailureRate once attempts ≥ 5 (§7)
  │
  ├─ per CrawledPage: AllChecks() → ScorePage → PageReport
  ├─ FindDuplicateTitles + FindBrokenLinks → SiteResult
  ├─ report.BuildReport → WriteJSON / WriteText (schema_version: 1)
  │
  └─ map Crawl err → exit 2; else write report; --fail-below → exit 0 or 1
     (--baseline regression → Stage 10)
```

---



## 13. Decisions Log (Summary Table)


| #   | Decision                                   | Value / Approach                                                                                                                                                                                              |
| --- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Overall crawl time budget                  | New `--max-duration` flag, default 5m, becomes root context deadline                                                                                                                                          |
| 2   | Timeout granularity                        | Transport phase timeouts fixed/conservative (dial 5s, TLS 5s); per-page timeout fixed at 15s base but **escalates on retry** (×1.5), capped at **90s** total per page, **enforced via** `context.WithTimeout` |
| 3   | Retries                                    | Enabled, max 2 retries (3 attempts), exponential backoff, only for timeout/connection/429/5xx errors — **not** DNS-not-found, TLS/cert errors, or redirect loops                                              |
| 4   | Exit-code-2 boundary                       | Root URL unreachable, or >50% page failure rate with ≥5 pages attempted (checked **incrementally**, not only at crawl end); individual page failures scored normally                                          |
| 5   | Cancellation behavior                      | Hard-exit, no partial report, on root context cancellation (deadline, Ctrl+C, **or SIGTERM**); cause reported on stderr                                                                                       |
| 6   | Body size cap                              | 2 MB default, via `--max-body-size` flag; truncation detected via a `cap+1`-byte read (`PageResponse.Truncated`), recorded as a check finding later; applies post-decompression                               |
| 7   | Crawl concurrency                          | **Sequential for v1** (no `--workers` flag); `MaxConnsPerHost` / `MaxIdleConnsPerHost` = 1. Deterministic BFS truncation under `--max-pages`. Parallel fetches deferred to v2+ only with level-synchronous admission |
| —   | `Client.Timeout` / `ResponseHeaderTimeout` | Neither set — per-page context deadlines supersede both entirely (fixing a bug where `ResponseHeaderTimeout: 15s` silently capped every retry attempt)                                                        |
| —   | Redirects                                  | Capped at 5 hops; loop error is a sentinel (`ErrTooManyRedirects`) classified as non-retryable                                                                                                                |
| —   | Rate limiter                               | Context-aware (`select` on `ctx.Done()`) inter-request pacing delay; effective rate ≈ `1 / delay`                                                                                                             |
| —   | robots.txt                                 | Own fixed 10s timeout, fail-open on failure; `Crawl-delay` parsed but not applied to pacing in v1; shared User-Agent with Fetcher                                                                              |
| —   | Frontier                                   | Public `Crawl(ctx, *Fetcher, …)` free function; root-then-robots-then-BFS; `seen` via `Normalize`; stop when queue empty or `--max-pages` reached                                                              |
| —   | `NewClient`                                | No `Config` arg — transport knobs are fixed; UA / body / retry belong on `Fetcher`                                                                                                                            |


---
