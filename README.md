# seo-audit

A Go CLI that crawls a site, runs SEO and technical-quality checks, scores it
0–100, and fails when quality drops. It runs two ways:

- **Standalone:** human-readable terminal output for spot-checking a site.
- **In CI**: JSON report on stdout plus stable exit codes, so a CI runner can use it as a quality gate and compare each run against a stored baseline.

Everything the CI side depends on: exit codes, JSON schema, baseline semantics, is documented in `[docs/](docs)`.

## Contents

- [Install](#install)
- [Examples](#examples)
- [Commands](#commands)
- [Checks](#checks)
- [Scoring](#scoring)
- [Exit codes](#exit-codes)
- [CI usage](#ci-usage-in-development)
- [JSON report](#json-report)
- [How the crawl works](#how-the-crawl-works)
- [Project layout](#project-layout)
- [Development](#development)
- [Scope](#scope)
- [Documentation](#documentation)
- [License](#license)



## Install

```bash
go install github.com/Erose112/seo-audit@latest
```

Or build from a clone (requires Go 1.27+):

```bash
make build          # -> bin/seo-audit, with version/commit/date baked in
seo-audit --version
```

For release archives across Linux/Windows (amd64 + arm64):

```bash
make dist           # -> dist/seo-audit_<version>_<os>_<arch>.tar.gz
```



## Examples

Audit a site and print a report:

```bash
seo-audit crawl --url https://example.com --max-pages 25
```

```text
SEO AUDIT
━━━━━━━━━━━━━━━━━━━━━━━━━━
✓ Canonical link present
✓ Images have alt attributes
✓ Meta description present and well-sized
✗ Exactly one H1
✓ Title tag exists
⚠ Title length within range
✓ Responsive viewport meta tag

Score: 82/100

FAILED CHECKS
- /pricing
  Multiple H1 tags
  Title is 18 characters (recommended: 30-60)

SITE ISSUES
- Duplicate title "Home" on:
  /
  /index.html
```

Machine-readable output, gated on a minimum score:

```bash
seo-audit crawl --url https://example.com --output json --fail-below 80 > report.json
```

Gate on regressions against the previous run:

```bash
seo-audit crawl --url https://example.com --output json \
  --fail-below 80 --baseline ./latest.json
```

The first run seeds `latest.json`; every later run compares against it, writes
the regression summary to **stderr**, then overwrites the baseline.

## Commands



### `crawl`

Crawls a site, runs all checks, scores it, and optionally gates on score and
regression.


| Flag               | Default                         | Description                                                                                                                                                                    |
| ------------------ | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--url`            | *(required)*                    | Root URL to crawl.                                                                                                                                                             |
| `--output`         | `text`                          | `text` for humans, `json` for CI. JSON goes to stdout with nothing else mixed in.                                                                                              |
| `--max-pages`      | `100`                           | Crawl budget.                                                                                                                                                                  |
| `--max-depth`      | `5`                             | Link-depth limit from the root.                                                                                                                                                |
| `--delay`          | `200ms`                         | Delay between requests (politeness).                                                                                                                                           |
| `--max-duration`   | `5m`                            | Overall crawl timeout.                                                                                                                                                         |
| `--max-body-size`  | `2097152`                       | Response bytes read per page (2 MiB).                                                                                                                                          |
| `--fail-below`     | `0`                             | Exit 1 when the site score is below this. **The default disables the score gate** (`score < 0` is never true).                                                                 |
| `--baseline`       | *(none)*                        | Path to a baseline JSON report. Enables regression comparison and baseline write-back. **Omitting it disables both.**                                                          |
| `--max-score-drop` | `5`                             | Tolerated score drop vs the baseline. A drop of exactly this much passes.                                                                                                      |
| `--strict-rules`   | see [below](#regression-gating) | SEO checks (e.g. `SINGLE_H1`) that fail the build on new or worse failures even when the score drop is within `--max-score-drop`. Pass `""` to disable (score-drop gate only). |




### `compare`

Diffs two saved reports without crawling anything; useful for debugging a gate failure after the fact.

```bash
seo-audit compare --baseline old.json --current new.json
```

```text
SEO REGRESSION DETECTED

Score:
88 → 74 (-14)

New failures:
✗ /pricing — No <h1> found

Blocking: score dropped 14 (max 5); strict rules SINGLE_H1

Build failed.
```

Flags: `--baseline` and `--current` (both required), plus `--fail-below`,
`--max-score-drop`, and `--strict-rules` with the same meaning as `crawl`.
Unlike `crawl`, the diff is written to stdout. Exits 1 on regression or a score
below `--fail-below`.

## Checks

Seven per-page checks, plus two site-wide findings that need the whole crawl.


| Check ID           | What it requires                                         | Failure severity and deduction                                                                             |
| ------------------ | -------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `TITLE_EXISTS`     | A non-empty `<title>`                                    | error, −10                                                                                                 |
| `TITLE_LENGTH`     | Title of 30–60 characters                                | warning, −5 (skipped when the title is missing, so one problem is not charged twice)                       |
| `META_DESCRIPTION` | A meta description of 50–160 characters                  | warning, −5 missing / −3 out of range                                                                      |
| `SINGLE_H1`        | Exactly one `<h1>`                                       | error, −10 when none; warning, −5 when several                                                             |
| `IMAGE_ALT`        | Every `<img>` has an `alt` attribute                     | scaled with the share missing: `round(10 × pct)`, minimum −1; warning below 50% missing, error at or above |
| `CANONICAL`        | A `<link rel="canonical">`                               | warning, −5                                                                                                |
| `VIEWPORT`         | `<meta name="viewport">` containing `width=device-width` | error, −10 when absent; warning, −5 when present but missing the directive                                 |
| `DUPLICATE_TITLE`  | Titles unique across the site                            | reported, no score impact — enforced via strict rules                                                      |
| `BROKEN_LINK`      | Internal links resolve successfully                      | reported, no score impact — enforced via strict rules                                                      |


Lengths are counted in characters, not bytes, so accented, CJK, and emoji
titles are not penalized for their encoding. `alt=""` passes: it is the
intentional decorative-image pattern, not a missing attribute.

Thresholds are named constants in `internal/checks` because tuning them against
real-world pages is a deliberate follow-up, not a config surface.

## Scoring

A page starts at 100, subtracts every check's deduction, and clamps to 0–100. The site score is the floored average of page scores.

Deductions live on the checks themselves rather than in a severity lookup
table, for two reasons: the score stays reconstructable from the JSON report
(which serializes each `deduction`), and checks like `IMAGE_ALT` can express
magnitude instead of a single flat penalty.

Averaging dilutes: on a 100-page site, one page collapsing from 100 to 0 moves
the site score by a single point. That is why structural regressions are
enforced by strict rules rather than by the score gate alone.

## Exit codes

A contract with the CI runner see [docs/ci-integration.md](docs/ci-integration.md).


| Code | Meaning                                                                                                                                   | What to do                                                                                  |
| ---- | ----------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `0`  | Passed both gates                                                                                                                         | Nothing. stdout holds a valid report.                                                       |
| `1`  | Quality gate failure — score below `--fail-below`, or a regression                                                                        | Fix the site or content. stdout still holds a valid report; stderr explains the regression. |
| `2`  | Crawl or system error — unreachable root URL, over 50% fetch failures, cancellation, corrupt baseline, unknown strict rule, invalid flags | Fix infrastructure or config. **Do not** parse stdout.                                      |


Exit 1 is a content problem; exit 2 means the tool never got a trustworthy look
at the site.

## CI usage (In-development)

The tool has no environment-variable or job-directory awareness. The runner
passes an explicit `--baseline` path into a directory it keeps between runs:

```yaml
seo-audit:
  command: seo-audit
  args:
    - crawl
    - "--url"
    - "https://example.com"
    - "--output"
    - "json"
    - "--fail-below"
    - "80"
    - "--baseline"
    - "<job-dir>/latest.json"
    - "--max-pages"
    - "100"
    - "--max-duration"
    - "5m"
```

Requirements on the runner side:

- **stdout carries only the report.** Regression output, crawl errors, and
baseline-save warnings go to stderr. Don't merge the streams if you parse the
JSON.
- **The job directory must exist, be writable, and persist** across runs.
`SaveBaseline` writes `latest.json.tmp` then renames, so both paths must be
on the same filesystem, and it does not create parent directories.
- **Set the job timeout above** `--max-duration` and send SIGTERM rather than
SIGKILL, so the crawl cancels gracefully mid-write.
- A failed crawl (exit 2) never writes a baseline, so a broken run cannot
poison the comparison for the next one.

Simulate the two-run baseline flow locally:

```bash
make ci-sim URL=https://example.com
```



### Regression gating

A run is a regression when either condition holds:

```text
score drop > --max-score-drop   OR   any new failure or escalation hits a strict rule
```

Strict rules default to the template-level checks: `TITLE_EXISTS`,
`SINGLE_H1`, `VIEWPORT`, `CANONICAL`, `BROKEN_LINK`, `DUPLICATE_TITLE`. These
are rendered by code and essentially never fail on purpose.

`TITLE_LENGTH`, `META_DESCRIPTION`, and `IMAGE_ALT` are deliberately advisory. They fire constantly from routine content edits, and so instead of blocking deploys on every new title-length check, it is served as a warning.

The comparison also catches **escalation**: a check already failing that gets
worse (severity rank or deduction increased) without being a new failure.
Baseline failures on pages that were not crawled this run are skipped rather
than reported as fixed. Full policy in
[docs/regression-engine.md](docs/regression-engine.md).

## JSON report

`--output json` emits a single deterministic object with a trailing newline. Map keys are ordered, so two identical runs produce identical bytes. A baseline file is the same document shape; there is no separate baseline schema.

Full field reference, enum values, and stability policy:
[docs/report-schema.md](docs/report-schema.md).

## How the crawl works

Sequential breadth-first traversal from the root URL, bounded by `--max-pages`,
`--max-depth`, and `--max-duration`. The crawler honors robots.txt, retries with escalating timeouts, classifies fetch failures, and aborts with exit 2 when the site is mostly unreachable. Pages are deduplicated via `crawler.Normalize` (fragments stripped, scheme/host lowercased; path casing, trailing slashes, and query strings left distinct).

Design notes, retry schedule, error classification, and URL-normalization edge
cases: [docs/crawler-architecture.md](docs/crawler-architecture.md).

## Project layout

```text
cmd/                    Cobra commands (crawl, compare) and exit-code mapping
internal/crawler/       HTTP fetch, retries, URL frontier, robots.txt, rate limiting
internal/parser/        HTML -> PageData
internal/checks/        Check implementations and site-wide analysis
internal/scoring/       Check results -> 0-100 score
internal/report/        Text and JSON report generation
internal/regression/    Baseline I/O and comparison
internal/config/        Typed CLI config passed down to internal packages
internal/buildinfo/     Version/commit/date baked in at build time
```

Data flows one direction. No later stage reaches back into an earlier one's
internals, which is what keeps everything from the parser onward testable with
no network access.

## Development

```bash
make test               # unit tests
make test-integration   # subprocess tests asserting the 0/1/2 exit-code contract
make vet
make fmt-check
make help               # all targets
```

CI runs vet, build, and both test suites on every push and pull request to
`main`.

## Scope

**In v1:** the nine checks above, sequential crawling with robots.txt and rate
limiting, 0–100 scoring, text and JSON reports, file-based baseline regression,
and the CI exit-code contract.

**Deliberately out of v1:** full schema.org validation, JavaScript-rendered page auditing, sitemap intelligence, redirect-chain tracking, orphan-page analysis, duplicate meta-description detection, a score-history dashboard, and concurrent scraping. Baseline storage stays a plain JSON file; no database or caching.

## Documentation


| Document                                                | Contents                                                                         |
| ------------------------------------------------------- | -------------------------------------------------------------------------------- |
| [ci-integration.md](docs/ci-integration.md)             | Runner contract: job fields, exit codes, job-directory semantics, stream routing |
| [report-schema.md](docs/report-schema.md)               | JSON report field reference and stability policy                                 |
| [regression-engine.md](docs/regression-engine.md)       | Diff model, gating policy, escalation, baseline lifecycle                        |
| [crawler-architecture.md](docs/crawler-architecture.md) | Crawl strategy, retry and error-classification design                            |
| [development-plan.md](docs/development-plan.md)         | Architecture overview and original design goals                                  |
| [development-timeline.md](docs/development-timeline.md) | Staged build plan and status                                                     |




## License

[MIT](LICENSE)