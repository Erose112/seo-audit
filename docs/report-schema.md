# SEO Audit Report JSON Schema

**Version:** 1 (`schema_version: 1`)

This document is the contract of record for the JSON report emitted by
`seo-audit crawl --output json`. The CI runner depends on this schema — treat
field names, types, and presence rules as an API contract.

Implementation uses Go's `encoding/json/v2` with `json.Deterministic(true)` so
map key order is stable across runs.

## Baseline files

`crawl --baseline <path>` reads and writes the same JSON document shape as
`crawl --output json`. There is no separate baseline schema — a baseline file
is a full report. See [regression-engine.md](regression-engine.md) for the
comparison and gating policy.

## Changelog

### Version 1

Initial schema. Added during Stage 8 before any CI consumer existed:

- `schema_version` — contract version integer
- `site_issues` — per-URL duplicate-title and broken-link detail (not just counts)



## Top-level object: `Report`


| Field            | Type                    | Always present | Description                                     |
| ---------------- | ----------------------- | -------------- | ----------------------------------------------- |
| `schema_version` | integer                 | yes            | Contract version. Currently `1`.                |
| `url`            | string                  | yes            | Root URL passed to `crawl --url`.               |
| `timestamp`      | string (RFC 3339 UTC)   | yes            | Report generation time.                         |
| `score`          | integer                 | yes            | Site score 0–100 (average of page scores).      |
| `pages_crawled`  | integer                 | yes            | Number of successfully crawled pages.           |
| `checks`         | array of `CheckSummary` | yes            | Site-level check roll-up (empty array if none). |
| `pages`          | array of `PageReport`   | yes            | Per-page results (empty array if none).         |
| `site_issues`    | `SiteResult`            | yes            | Duplicate titles and broken links.              |
| `summary`        | `Summary`               | yes            | Aggregate counts.                               |




## `CheckSummary`


| Field          | Type    | Always present | Description                                            |
| -------------- | ------- | -------------- | ------------------------------------------------------ |
| `check_id`     | string  | yes            | Check identifier (e.g. `TITLE_EXISTS`).                |
| `name`         | string  | yes            | Human-readable check name.                             |
| `severity`     | string  | yes            | Worst severity across all pages. See severities below. |
| `pages_failed` | integer | yes            | Pages where this check is `warning` or `error`.        |
| `pages_total`  | integer | yes            | Pages where this check ran.                            |




## `PageReport`


| Field     | Type                   | Always present | Description                      |
| --------- | ---------------------- | -------------- | -------------------------------- |
| `url`     | string                 | yes            | Page URL.                        |
| `score`   | integer                | yes            | Page score 0–100.                |
| `results` | array of `CheckResult` | yes            | All check results for this page. |




## `CheckResult`


| Field       | Type    | Always present | Description                                           |
| ----------- | ------- | -------------- | ----------------------------------------------------- |
| `check_id`  | string  | yes            | Check identifier.                                     |
| `severity`  | string  | yes            | `pass`, `info`, `warning`, or `error`.                |
| `message`   | string  | yes            | Human-readable result message.                        |
| `deduction` | integer | yes            | Points deducted from 100 for this check (0 for pass). |




## `SiteResult`


| Field              | Type                  | Always present | Description                                                                                          |
| ------------------ | --------------------- | -------------- | ---------------------------------------------------------------------------------------------------- |
| `duplicate_titles` | object                | yes            | Map of title string → array of URLs sharing that title (2+ pages only). Empty object `{}` when none. |
| `broken_links`     | array of `BrokenLink` | yes            | Internal links whose target failed during crawl. Empty array `[]` when none.                         |




## `BrokenLink`


| Field         | Type    | Always present | Description                                                               |
| ------------- | ------- | -------------- | ------------------------------------------------------------------------- |
| `source_url`  | string  | yes            | Page containing the link.                                                 |
| `target_url`  | string  | yes            | Normalized target URL that failed.                                        |
| `kind`        | string  | yes            | Failure kind. See kinds below.                                            |
| `status_code` | integer | **no**         | HTTP status when `kind` is `http_status`. Omitted when zero (`omitzero`). |
| `message`     | string  | **no**         | Detail for parse failures and similar. Omitted when empty (`omitzero`).   |




## `Summary`


| Field               | Type    | Always present | Description                                                 |
| ------------------- | ------- | -------------- | ----------------------------------------------------------- |
| `pages_crawled`     | integer | yes            | Same as top-level `pages_crawled`.                          |
| `pages_with_errors` | integer | yes            | Crawl/fetch failures (including parse failures).            |
| `broken_links`      | integer | yes            | Count of entries in `site_issues.broken_links`.             |
| `duplicate_titles`  | integer | yes            | Count of duplicate title groups (map keys), not page count. |
| `average_score`     | integer | yes            | Same as top-level `score`.                                  |




## Enumerated values



### `severity`


| Value     | Meaning                                 |
| --------- | --------------------------------------- |
| `pass`    | Check passed.                           |
| `info`    | Informational finding, no score impact. |
| `warning` | Non-critical issue.                     |
| `error`   | Critical issue.                         |




### `kind` (broken link failure type)


| Value           | Meaning                                     |
| --------------- | ------------------------------------------- |
| `timeout`       | Request timed out.                          |
| `connection`    | Transient connection failure.               |
| `tls`           | TLS/certificate error.                      |
| `http_status`   | HTTP error response (see `status_code`).    |
| `permanent`     | Deterministic failure (DNS, redirect loop). |
| `canceled`      | Crawl canceled.                             |
| `parse_failure` | Fetched but not parseable HTML.             |




## Serialization semantics

These behaviors are load-bearing for CI parsing and golden tests:

- **Encoder:** `encoding/json/v2` with `json.Deterministic(true)` — map keys
(including `duplicate_titles`) marshal in sorted order.
- **Nil slices/maps:** v2 default emits `[]` and `{}`, not `null`. No manual
normalization is applied in `BuildReport`.
- **Optional fields:** `BrokenLink.status_code` and `BrokenLink.message` use
`omitzero` (omit on Go zero value). Under v2, `omitempty` on integers does
*not* omit zero — `omitzero` is required for the "meaningful when non-zero"
semantics.
- **Trailing newline:** `WriteJSON` appends a single `\n` after the JSON object.
- **Timestamps:** RFC 3339 UTC (e.g. `"2026-03-13T12:00:00Z"`).
- **Field matching on decode:** v2 matches JSON keys case-sensitively (stricter
than v1). Baseline loading uses `json.UnmarshalRead` from v2.



## Stability policy

Per project scope rules:

- **Additive changes** (new optional fields): update this changelog; do not bump
`schema_version` unless the CI runner requires it.
- **Breaking changes** (rename, remove, or change type of an existing field):
bump `schema_version` and document the migration here. Stop and flag before
shipping.



## Example payload

Generated from the same fixture as `testdata/expected_report.json`:

```json
{
  "schema_version": 1,
  "url": "https://example.com/",
  "timestamp": "2026-03-13T12:00:00Z",
  "score": 75,
  "pages_crawled": 2,
  "checks": [
    {
      "check_id": "CANONICAL",
      "name": "Canonical link present",
      "severity": "pass",
      "pages_failed": 0,
      "pages_total": 2
    },
    {
      "check_id": "IMAGE_ALT",
      "name": "Images have alt attributes",
      "severity": "warning",
      "pages_failed": 1,
      "pages_total": 2
    },
    {
      "check_id": "META_DESCRIPTION",
      "name": "Meta description present and well-sized",
      "severity": "pass",
      "pages_failed": 0,
      "pages_total": 2
    },
    {
      "check_id": "SINGLE_H1",
      "name": "Exactly one H1",
      "severity": "error",
      "pages_failed": 1,
      "pages_total": 2
    },
    {
      "check_id": "TITLE_EXISTS",
      "name": "Title tag exists",
      "severity": "pass",
      "pages_failed": 0,
      "pages_total": 2
    },
    {
      "check_id": "TITLE_LENGTH",
      "name": "Title length within range",
      "severity": "pass",
      "pages_failed": 0,
      "pages_total": 2
    },
    {
      "check_id": "VIEWPORT",
      "name": "Responsive viewport meta tag",
      "severity": "pass",
      "pages_failed": 0,
      "pages_total": 2
    }
  ],
  "pages": [
    {
      "url": "https://example.com/about",
      "score": 90,
      "results": [
        {
          "check_id": "TITLE_EXISTS",
          "severity": "pass",
          "message": "Title present",
          "deduction": 0
        },
        {
          "check_id": "SINGLE_H1",
          "severity": "pass",
          "message": "One H1",
          "deduction": 0
        },
        {
          "check_id": "IMAGE_ALT",
          "severity": "pass",
          "message": "All images have alt",
          "deduction": 0
        }
      ]
    },
    {
      "url": "https://example.com/home",
      "score": 60,
      "results": [
        {
          "check_id": "TITLE_EXISTS",
          "severity": "pass",
          "message": "Title present",
          "deduction": 0
        },
        {
          "check_id": "SINGLE_H1",
          "severity": "error",
          "message": "Multiple H1 tags",
          "deduction": 10
        },
        {
          "check_id": "IMAGE_ALT",
          "severity": "warning",
          "message": "Missing alt on 1 image",
          "deduction": 5
        }
      ]
    }
  ],
  "site_issues": {
    "duplicate_titles": {
      "Alpha Title": [
        "https://example.com/a",
        "https://example.com/b"
      ],
      "Beta Title": [
        "https://example.com/c",
        "https://example.com/d"
      ]
    },
    "broken_links": [
      {
        "source_url": "https://example.com/",
        "target_url": "https://example.com/missing",
        "kind": "http_status",
        "status_code": 404,
        "message": "404 Not Found"
      },
      {
        "source_url": "https://example.com/about",
        "target_url": "https://example.com/gone",
        "kind": "parse_failure"
      }
    ]
  },
  "summary": {
    "pages_crawled": 2,
    "pages_with_errors": 1,
    "broken_links": 2,
    "duplicate_titles": 2,
    "average_score": 75
  }
}
```

