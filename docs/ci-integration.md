# SEO Audit — CI Runner Integration

**Version:** 1

This document is the contract of record for how the Go CI runner invokes
`seo-audit crawl` as a quality gate. It covers the job entry fields the runner
must supply, how exit codes map to pass/fail behavior, and filesystem
requirements for baseline persistence.

The JSON report shape is defined separately in [report-schema.md](report-schema.md).
Regression gating policy is in [regression-engine.md](regression-engine.md).

## Changelog

### Version 1

Initial contract:

- Job entry field requirements and rationale
- Exit-code mapping for the runner
- Persistent job-directory semantics
- Stream routing (stdout vs stderr)

## Job entry fields

The runner invokes `seo-audit crawl` as a subprocess. Every field below is a
CLI flag — the tool has **no** env-var or job-dir awareness; the runner passes
an explicit `--baseline` path pointing into its persistent directory.

| Flag | Required | Typical value | Why |
| --- | --- | --- | --- |
| `--url` | yes | `https://example.com` | Root URL to crawl. |
| `--output` | yes (CI) | `json` | Machine-readable report on stdout. Use `text` only for human debugging. |
| `--fail-below` | yes (CI) | `80` | Exit 1 when site score is below this threshold. **Default `0` disables the score gate entirely** (`score < 0` is never true). |
| `--baseline` | yes (CI) | `<job-dir>/latest.json` | Enables regression comparison and baseline write-back. **Omitting this disables regression gating and prevents the baseline from ever being written.** |
| `--max-score-drop` | no | `5` | Tolerated score drop vs baseline before regression exit 1. |
| `--strict-rules` | no | (default list) | Check IDs whose *new* failures fail the gate regardless of score. Pass `""` to disable. |
| `--max-pages` | no | `100` | Crawl budget. Lower for faster CI on large sites. |
| `--max-duration` | no | `5m` | Wall-clock crawl ceiling. Runner job timeout must exceed this. |
| `--max-depth` | no | `5` | Link depth limit. |
| `--delay` | no | `200ms` | Inter-request delay (politeness). |

### Worked job definition

Extends the illustrative example in [development-plan.md](development-plan.md):

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

Replace `<job-dir>` with the runner's persistent per-job working directory
(see below). The runner is responsible for creating that directory before
invocation.

## Exit codes

| Code | Meaning | Runner action |
| --- | --- | --- |
| **0** | Pass — score gate and regression gate both satisfied | Mark job success. Optionally parse stdout JSON for dashboard metrics. |
| **1** | Quality gate failure — score below `--fail-below` **or** regression detected | Mark job failed (quality). Surface stderr in the log — it contains the `SEO REGRESSION DETECTED` block when applicable. stdout still holds a valid JSON report. |
| **2** | Infrastructure / tool failure — unreachable root URL, >50% fetch failure rate, crawl cancellation, corrupt baseline, unknown strict rule, invalid flags | Mark job failed (infrastructure). Surface stderr (`crawl error: …`). **Do not** treat stdout as a valid report. Remediation differs from exit 1 (e.g. corrupt baseline: delete `latest.json` and re-run). |

**Decision:** exit codes are a stable contract — do not remap them. Distinguish
exit 1 (fix the site/content) from exit 2 (fix infrastructure/config).

*Rationale:* the tool uses `os.Exit(1)` for gate failures and `os.Exit(2)` for
crawl/system errors inside `RunE`; returned errors from `Execute()` also map
to exit 2.

## Persistent job directory

The runner must provide a directory that:

1. **Exists and is writable before invocation** — `SaveBaseline` writes to
   `<path>.tmp` then renames; it does **not** create parent directories.
2. **Persists across runs** for the same logical job — not wiped between
   consecutive executions.
3. **Lives on one filesystem** — atomic rename requires source and destination
   on the same volume.

### Baseline lifecycle

1. **First run** — `<job-dir>/latest.json` missing; tool treats baseline as
   empty (no regression possible). After crawl, writes current report to the path.
2. **Subsequent runs** — loads existing file, compares after report write,
   overwrites with current report regardless of pass/fail.
3. **Corrupt baseline** — parse or schema mismatch fails fast (exit 2) before
   crawl starts.

### Baseline write failures

If `SaveBaseline` fails (read-only directory, disk full), the tool prints
`warning: failed to save baseline:` to stderr and **does not change the exit
code**. The runner should monitor stderr for this warning — without a saved
baseline, the next run cannot detect regressions.

## Stream routing

| Stream | Contents |
| --- | --- |
| **stdout** | The audit report only (`--output json` or `text`). Must remain pure JSON when `--output json`. |
| **stderr** | `crawl error:` messages, regression summary (`SEO REGRESSION DETECTED`), baseline save warnings. |

**Decision:** the runner must **not** merge stdout and stderr if it parses the
JSON report from stdout.

## Process lifecycle

- Set the runner job timeout **above** `--max-duration` (default 5 minutes) so
  the tool self-terminates with a clean cancellation message rather than being
  SIGKILL'd mid-write.
- Send **SIGTERM** (not SIGKILL) on timeout — `crawler.NewCrawlContext` traps
  SIGINT and SIGTERM for graceful shutdown.

## Guarantees on failure

| Scenario | Report on stdout | Baseline written |
| --- | --- | --- |
| Crawl systemic failure (exit 2) | No | No |
| Score/regression gate failure (exit 1) | Yes | Yes (if `--baseline` set) |
| Pass (exit 0) | Yes | Yes (if `--baseline` set) |

A broken crawl cannot poison the baseline — only successful crawls reach
`SaveBaseline`.

## Binary distribution

**Phase 1 (current):** build release archives locally with the Makefile:

```bash
make dist
```

This produces one `tar.gz` per platform under `dist/`:

- `seo-audit_<version>_linux_amd64.tar.gz`
- `seo-audit_<version>_linux_arm64.tar.gz`
- `seo-audit_<version>_windows_amd64.tar.gz`

Each archive contains a single `seo-audit` binary (with `.exe` on Windows).
Copy the matching archive to the runner host, extract, and place the binary
on `PATH` or reference it by absolute path in the job config.

Verify the build with `seo-audit --version` before invoking the job.

**Phase 2 (deferred):** automated GitHub Releases via goreleaser, with a
checksums file (`seo-audit_<version>_checksums.txt`) for pinned downloads.
Archive naming will stay the same so the runner config does not change.

## Stability policy

**Frozen (breaking change requires explicit coordination):**

- Exit codes 0 / 1 / 2 and their meanings
- JSON report `schema_version: 1` field set (see report-schema.md)

**Additive changes are OK:**

- New optional CLI flags with safe defaults
- New check IDs (may affect regression strict rules if added to defaults)

**Breaking changes require:**

- Bumping `schema_version` for report shape changes
- Announcing exit-code or flag semantic changes to all runner consumers

## Related documents

- [report-schema.md](report-schema.md) — JSON report field reference
- [regression-engine.md](regression-engine.md) — diff model and gating policy
- [development-plan.md](development-plan.md) — architecture overview
