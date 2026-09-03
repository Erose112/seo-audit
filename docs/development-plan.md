# SEO Audit CLI ΓÇö Development Plan

## Project Summary

Build **`seo-audit`**, a Go-based command-line SEO auditing tool that crawls a
website, evaluates SEO/technical-quality rules, generates a 0ΓÇô100 score, and
acts as a **CI quality gate**.

It supports two modes of use:

- **Human use** ΓÇö run an audit locally, get readable terminal output.
- **Automated use** ΓÇö the CI runner executes it on deploy, parses its JSON
  report, compares against a baseline, and fails the build on regression.

The project connects two pieces of engineering work:

> **Go CI runner ΓåÆ executes ΓåÆ Go SEO auditing tool ΓåÆ produces structured
> result ΓåÆ runner decides whether the build passes.**

### Core Tech Stack

| Area | Technology |
|---|---|
| Language | Go |
| CLI framework | Cobra (`spf13/cobra`) |
| HTML parsing | `golang.org/x/net/html` or GoQuery |
| HTTP | Go standard `net/http` |
| `robots.txt` | Go robots parser library |
| Rate limiting | Go timer/ticker |
| Output | Terminal text + JSON |
| Terminal formatting | `fatih/color` |
| Baseline storage | Plain JSON file in a persistent per-job directory (see below) |
| CI integration | Existing Go CI runner |
| Testing | Go `testing`, table-driven tests |
| Deployment targets | QRET website / Degree Planner |

### Scope discipline

Keep v1 deliberately narrow: a reliable crawler, a small set of well-chosen
checks, deterministic scoring, JSON reporting, regression detection, and CI
integration on one real site. Full schema.org validation, JS-rendered pages,
sitemap intelligence, redirect-chain tracking, orphan-page detection, and any
LLM-based suggestion layer are explicitly **out of scope for v1** ΓÇö see
"Deferred / v2" at the end. The goal is a small tool that actually gets
deployed and used, not a comprehensive one that doesn't.

---

## Architecture

```text
CLI
 Γöé
 Γö£ΓöÇΓöÇ Configuration
 Γöé
 Γö£ΓöÇΓöÇ Crawler
 Γöé    Γö£ΓöÇΓöÇ URL frontier
 Γöé    Γö£ΓöÇΓöÇ robots.txt
 Γöé    Γö£ΓöÇΓöÇ rate limiter
 Γöé    ΓööΓöÇΓöÇ HTTP client
 Γöé
 Γö£ΓöÇΓöÇ Parser
 Γöé    ΓööΓöÇΓöÇ HTML ΓåÆ PageData
 Γöé
 Γö£ΓöÇΓöÇ Checks
 Γöé    Γö£ΓöÇΓöÇ On-page
 Γöé    Γö£ΓöÇΓöÇ Technical
 Γöé    ΓööΓöÇΓöÇ Content
 Γöé
 Γö£ΓöÇΓöÇ Scoring Engine
 Γöé
 Γö£ΓöÇΓöÇ Report Generator
 Γöé    Γö£ΓöÇΓöÇ Text
 Γöé    ΓööΓöÇΓöÇ JSON
 Γöé
 ΓööΓöÇΓöÇ Regression Engine
      ΓööΓöÇΓöÇ Baseline comparison (file-based)
```

---

## Requirements

**Functional**
- URL supplied via CLI
- Crawl internal pages, obeying `robots.txt`
- Configurable page limit and crawl rate
- Extract SEO metadata, run checks, calculate score
- Text and JSON report output
- Meaningful exit codes
- Compare against a baseline and detect regressions
- Run non-interactively in CI

**Non-functional**
- Deterministic scoring (same input ΓåÆ same score, every run)
- Reasonable crawl speed and memory use
- Clear error handling for crawl failures
- Checks implemented as an extensible framework, not hardcoded logic

---

## CLI Foundation

Project layout:

```text
seo-audit/
Γö£ΓöÇΓöÇ cmd/
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

Initial commands: `seo-audit crawl`, `seo-audit compare`

Core flags: `--url`, `--max-pages`, `--fail-below`, `--output`, `--baseline`,
`--max-depth`, `--delay`

Exit-code contract:
```text
0 = audit passed
1 = SEO gate failed (score below threshold, or regression detected)
2 = crawl/system error
```

---

## Single-page Crawler

- **HTTP client**: GET requests, timeout, redirects, user-agent, status-code
  handling, content-type validation.
- **URL normalization**: relative/absolute URLs, fragments, trailing slashes,
  query params, duplicate detection.
- **Fetch abstraction**: `Fetch(url) -> PageResponse`
- **HTML ΓåÆ PageData**: title, meta description, headings, images, canonical,
  viewport, links, word count.
- **Parser tests**: fixtures for a valid page, missing title, multiple H1s,
  missing alt text, malformed HTML, missing canonical, no viewport.

---

## SEO Check Engine

Design as an extensible framework rather than hardcoding checks into the
crawler:

```go
type Check interface {
    ID() string
    Name() string
    Run(PageData) CheckResult
}
```

**v1 checks (~6ΓÇô8 total):**
- Title exists / title length
- Meta description exists / length
- Exactly one H1
- Image alt-text coverage
- Canonical tag exists
- Viewport tag exists
- Duplicate titles across the site
- Broken internal links

Each check reports a severity (`PASS` / `WARNING` / `ERROR`) and a score
deduction, e.g.:

```text
TITLE_LENGTH
severity: warning
deduction: 5
```

---

## Scoring Engine

```text
100 points ΓåÆ deductions ΓåÆ final score
```

| Severity | Example deduction |
|---|---:|
| Error | -10 |
| Warning | -5 |
| Info | 0 |

Make weighting configurable rather than embedded throughout the codebase.
Site score = average of page scores (weighting important pages differently is
a later refinement, not v1).

Tests: perfect score, multiple failures, score clamped to 0ΓÇô100, deterministic
output.

---

## Crawl Engine

- **Frontier**: BFS queue ΓÇö fetch, parse links, normalize, check visited,
  enqueue if new.
- **Internal-link detection**: only crawl the target domain.
- **Visited tracking**: prevent duplicate crawling and cycles.
- **Depth and page limits**: `--max-depth`, `--max-pages`.
- **Rate limiting**: `--delay` between requests.
- **robots.txt**: fetch, parse, respect disallow rules, fail safely if
  unreachable.
- **Error handling**: distinguish 404 / 403 / 429 / 500 / timeout / connection
  failure / invalid HTML.

---

## Site-wide Analysis (v1 scope)

Keep this minimal for v1:
- Duplicate titles across pages
- Broken internal links (via a simple link graph)
- Site-level aggregation summary (pages crawled, pages with errors, broken
  links, duplicate titles, average score)

Duplicate meta descriptions, orphan-page analysis, and redirect-chain
detection are deferred (see "Deferred / v2").

---

## Reporting System

**Stable JSON schema** ΓÇö this matters because the CI runner depends on it, so
settle it before building CI integration:

```json
{
  "url": "https://example.com",
  "timestamp": "...",
  "score": 87,
  "pages_crawled": 42,
  "checks": [],
  "pages": [],
  "summary": {}
}
```

**Terminal output** (human-readable):

```text
SEO AUDIT
ΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöüΓöü
Γ£ô Title length
Γ£ô Meta description
Γ£ù Multiple H1 tags
ΓÜá Missing image alt text

Score: 87/100

FAILED CHECKS
- /about
  Multiple H1 tags
```

Keep diagnostic logs separate from JSON output so CI can parse the report
without log noise mixed in.

---

## Exit-code / CI-gate Behavior

```bash
seo-audit crawl \
  --url https://qret-website.vercel.app \
  --fail-below 80
```

```text
score >= threshold ΓåÆ 0
score < threshold  ΓåÆ 1
crawl failure      ΓåÆ 2
```

---

## Regression Comparison

This is what turns the tool from an SEO checker into an SEO regression gate.

- **Baseline representation**: the previous run's JSON report, passed via
  `--baseline previous-report.json`.
- **Score comparison**: `previous: 91, current: 84, delta: -7`
- **Regression threshold**: `--max-score-drop 5`
- **Check-level comparison**: detect specific checks flipping from PASS to
  FAIL, not just an overall score drop.
- **New vs. existing failures**: distinguish a pre-existing problem from a
  regression introduced by this change ΓÇö this distinction is what makes the
  gate useful in CI instead of just noisy.

Example output:
```text
SEO REGRESSION DETECTED

Score:
91 ΓåÆ 84 (-7)

New failures:
Γ£ù /about ΓÇö missing canonical
Γ£ù /programs ΓÇö duplicate title

Build failed.
```

### Baseline storage decision

Don't extend the CI runner's SQLite schema for this in v1. Instead:

1. The CI runner guarantees a **persistent working directory per job** that
   isn't wiped between runs (this is a small, general-purpose feature worth
   adding to the runner regardless of this project).
2. `seo-audit` reads `<job-dir>/latest.json` as the baseline at the start of
   a run (if it exists), and overwrites it with the current report at the
   end.
3. No database schema changes, no coupling between the two projects ΓÇö the
   SEO tool doesn't need to know anything about the runner's internals.

**Deferred to v2 (optional):** a generic `job_artifacts` table in the
runner's SQLite that any job can write to, so the dashboard can show score
history/trends over time rather than just latest-vs-current. Scope this as
an extension to the *CI runner* project specifically, not a dependency of
the SEO tool's v1.

---

## CI Runner Integration

Example job definition:
```yaml
seo-audit:
  command: seo-audit crawl
  args:
    - "--url"
    - "https://qret-website.vercel.app"
    - "--fail-below"
    - "80"
```

- **Binary distribution**: ship a versioned release binary the runner can
  fetch, rather than building Go from source on every run.
- **stdout/stderr**: streamed through the runner's existing log pipeline.
- **Exit code mapping**: 0 ΓåÆ success, 1 ΓåÆ failed quality gate, 2 ΓåÆ
  infrastructure/tool failure.
- **Report persistence**: written to the per-job directory described above.
- **Dashboard integration**: surface score, previous score, delta, and
  failing checks ΓÇö not just a pass/fail marker.

---

## Production Validation

- Run against QRET and Degree Planner; review crawl coverage, false
  positives, robots behavior, scoring sanity.
- Basic performance check: pages/second, memory use, crawl time ΓÇö enough to
  confirm it won't time out or blow up memory on a real site.
- Failure testing: intentionally introduce a broken link, missing title,
  duplicate title, missing canonical, bad H1 structure ΓÇö confirm the CI gate
  catches each one.
- Set up a nightly scheduled run (independent of deploys) to catch
  regressions from CMS/content changes, not just code changes.
- Document: installation, CLI usage, checks, scoring, exit codes, JSON
  schema, CI configuration, how to add a new check, baseline behavior.

---

## Deferred / v2

Explicitly out of scope for v1:
- Full schema.org / JSON-LD semantic validation
- JavaScript-rendered page auditing
- Sitemap intelligence
- Redirect-chain tracking
- Orphan-page analysis
- Duplicate meta-description detection
- Score history/trend dashboard (requires the `job_artifacts` SQLite
  extension to the CI runner, described above)

No LLM/AI-suggestion layer is planned for this project.

---

## Final Architecture

```text
                    ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ
                    Γöé   Cobra CLI  Γöé
                    ΓööΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓö¼ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÿ
                           Γöé
                    ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓû╝ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ
                    Γöé   Crawler    Γöé
                    Γöé HTTP         Γöé
                    Γöé robots.txt   Γöé
                    Γöé rate limit   Γöé
                    Γöé frontier     Γöé
                    ΓööΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓö¼ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÿ
                           Γöé
                    ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓû╝ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ
                    Γöé    Parser    Γöé
                    Γöé    HTML      Γöé
                    ΓööΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓö¼ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÿ
                           Γöé
                    ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓû╝ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ
                    Γöé    Checks    Γöé
                    Γöé On-page      Γöé
                    Γöé Technical    Γöé
                    Γöé Content      Γöé
                    ΓööΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓö¼ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÿ
                           Γöé
                    ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓû╝ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ
                    Γöé   Scoring    Γöé
                    ΓööΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓö¼ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÿ
                           Γöé
              ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓö┤ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ
              Γöé                         Γöé
       ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓû╝ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ          ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓû╝ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ
       Γöé Text Report Γöé          Γöé JSON Report Γöé
       ΓööΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÿ          ΓööΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓö¼ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÿ
                                       Γöé
                               ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓû╝ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ
                               Γöé   Regression   Γöé
                               Γöé    Engine      Γöé
                               Γöé (file-based    Γöé
                               Γöé  baseline)     Γöé
                               ΓööΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓö¼ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÿ
                                       Γöé
                               ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓû╝ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ
                               Γöé   Go CI Runner Γöé
                               ΓööΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓö¼ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÿ
                                       Γöé
                            ΓöîΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓû╝ΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÉ
                            Γöé Build Pass / Failed Γöé
                            ΓööΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÇΓöÿ
```
