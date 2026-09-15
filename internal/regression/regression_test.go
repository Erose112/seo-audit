package regression

import (
	"testing"

	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/Erose112/seo-audit/internal/crawler"
	"github.com/Erose112/seo-audit/internal/report"
)

func defaultOpts() Options {
	return Options{MaxScoreDrop: 5, StrictRules: DefaultStrictRules()}
}

func pageReport(url string, results ...checks.CheckResult) report.PageReport {
	return report.PageReport{URL: url, Score: 100, Results: results}
}

func result(checkID string, sev checks.Severity, msg string, ded int) checks.CheckResult {
	return checks.CheckResult{CheckID: checkID, Severity: sev, Message: msg, Deduction: ded}
}

func TestCompareFirstRun(t *testing.T) {
	t.Parallel()
	cur := report.Report{SchemaVersion: 1, Score: 80, Pages: []report.PageReport{
		pageReport("https://example.com/", result("CANONICAL", checks.Warning, "missing", 5)),
	}}
	got := Compare(report.Report{}, cur, defaultOpts())
	if got.Regressed {
		t.Error("first run should not regress")
	}
	if len(got.NewFailures) != 0 {
		t.Errorf("NewFailures = %d, want 0", len(got.NewFailures))
	}
}

func TestCompareImprovedScore(t *testing.T) {
	t.Parallel()
	base := report.Report{SchemaVersion: 1, Score: 80}
	cur := report.Report{SchemaVersion: 1, Score: 90}
	got := Compare(base, cur, defaultOpts())
	if got.Regressed || got.Delta != 10 {
		t.Errorf("got Regressed=%v Delta=%d", got.Regressed, got.Delta)
	}
}

func TestCompareDegradedUnderThreshold(t *testing.T) {
	t.Parallel()
	base := report.Report{SchemaVersion: 1, Score: 90}
	cur := report.Report{SchemaVersion: 1, Score: 86}
	got := Compare(base, cur, defaultOpts())
	if got.Regressed {
		t.Error("drop of 4 should be under threshold of 5")
	}
}

func TestCompareDegradedOverThreshold(t *testing.T) {
	t.Parallel()
	base := report.Report{SchemaVersion: 1, Score: 91}
	cur := report.Report{SchemaVersion: 1, Score: 84}
	got := Compare(base, cur, defaultOpts())
	if !got.ScoreDropExceeded || !got.Regressed {
		t.Errorf("ScoreDropExceeded=%v Regressed=%v", got.ScoreDropExceeded, got.Regressed)
	}
}

func TestCompareBoundaryAtThreshold(t *testing.T) {
	t.Parallel()
	base := report.Report{SchemaVersion: 1, Score: 100}
	cur := report.Report{SchemaVersion: 1, Score: 95}
	got := Compare(base, cur, defaultOpts())
	if got.ScoreDropExceeded {
		t.Error("Delta == -maxScoreDrop should not exceed threshold (strict <)")
	}
}

func TestCompareStrictNewFailure(t *testing.T) {
	t.Parallel()
	base := report.Report{
		SchemaVersion: 1,
		Score:         100,
		Pages:         []report.PageReport{pageReport("https://example.com/")},
	}
	cur := report.Report{
		SchemaVersion: 1,
		Score:         100,
		Pages: []report.PageReport{
			pageReport("https://example.com/about", result("CANONICAL", checks.Warning, "missing canonical", 5)),
		},
	}
	got := Compare(base, cur, defaultOpts())
	if !got.Regressed || len(got.StrictViolations) != 1 {
		t.Fatalf("Regressed=%v StrictViolations=%v", got.Regressed, got.StrictViolations)
	}
}

func TestCompareAdvisoryNewFailure(t *testing.T) {
	t.Parallel()
	base := report.Report{
		SchemaVersion: 1,
		Score:         100,
		Pages:         []report.PageReport{pageReport("https://example.com/")},
	}
	cur := report.Report{
		SchemaVersion: 1,
		Score:         100,
		Pages: []report.PageReport{
			pageReport("https://example.com/about", result("TITLE_LENGTH", checks.Warning, "too short", 5)),
		},
	}
	got := Compare(base, cur, defaultOpts())
	if len(got.NewFailures) != 1 {
		t.Fatalf("NewFailures = %d, want 1", len(got.NewFailures))
	}
	if got.Regressed {
		t.Error("author-level new failure should be advisory")
	}
}

func TestCompareEmptyStrictRulesScoreOnly(t *testing.T) {
	t.Parallel()
	base := report.Report{SchemaVersion: 1, Score: 100}
	cur := report.Report{
		SchemaVersion: 1,
		Score:         100,
		Pages: []report.PageReport{
			pageReport("https://example.com/", result("CANONICAL", checks.Warning, "missing", 5)),
		},
	}
	opts := Options{MaxScoreDrop: 5, StrictRules: nil}
	got := Compare(base, cur, opts)
	if got.Regressed {
		t.Error("empty strict rules should not regress on new failure alone")
	}
}

func TestCompareSimultaneousFlip(t *testing.T) {
	t.Parallel()
	base := report.Report{
		SchemaVersion: 1,
		Score:         90,
		Pages: []report.PageReport{
			pageReport("https://example.com/a", result("CANONICAL", checks.Warning, "missing", 5)),
			pageReport("https://example.com/b"),
		},
	}
	cur := report.Report{
		SchemaVersion: 1,
		Score:         90,
		Pages: []report.PageReport{
			pageReport("https://example.com/a"),
			pageReport("https://example.com/b", result("CANONICAL", checks.Warning, "missing", 5)),
		},
	}
	got := Compare(base, cur, defaultOpts())
	if len(got.NewFailures) != 1 || len(got.Resolved) != 1 {
		t.Fatalf("NewFailures=%d Resolved=%d", len(got.NewFailures), len(got.Resolved))
	}
	if !got.Regressed {
		t.Error("new strict failure should regress")
	}
}

func TestComparePageOnlyInBaselineNotResolved(t *testing.T) {
	t.Parallel()
	base := report.Report{
		SchemaVersion: 1,
		Score:         90,
		Pages: []report.PageReport{
			pageReport("https://example.com/gone", result("CANONICAL", checks.Warning, "missing", 5)),
		},
	}
	cur := report.Report{SchemaVersion: 1, Score: 95, Pages: []report.PageReport{
		pageReport("https://example.com/"),
	}}
	got := Compare(base, cur, defaultOpts())
	if len(got.Resolved) != 0 {
		t.Errorf("vanished page should not count as resolved, got %v", got.Resolved)
	}
}

func TestCompareDuplicateTitle(t *testing.T) {
	t.Parallel()
	base := report.Report{SchemaVersion: 1, Score: 100}
	cur := report.Report{
		SchemaVersion: 1,
		Score:         100,
		SiteIssues: checks.SiteResult{
			DuplicateTitles: map[string][]string{
				"Home": {"https://example.com/", "https://example.com/dup"},
			},
		},
	}
	got := Compare(base, cur, defaultOpts())
	if !got.Regressed {
		t.Fatal("new duplicate title should regress")
	}
}

func TestCompareBrokenLink(t *testing.T) {
	t.Parallel()
	base := report.Report{SchemaVersion: 1, Score: 100}
	cur := report.Report{
		SchemaVersion: 1,
		Score:         100,
		SiteIssues: checks.SiteResult{
			BrokenLinks: []checks.BrokenLink{{
				SourceURL: "https://example.com/",
				TargetURL: "https://example.com/missing",
				Kind:      crawler.ErrKindHTTPStatus,
				StatusCode: 404,
			}},
		},
	}
	got := Compare(base, cur, defaultOpts())
	if !got.Regressed || len(got.NewFailures) != 1 {
		t.Fatalf("Regressed=%v NewFailures=%v", got.Regressed, got.NewFailures)
	}
}

func TestCompareEscalationSingleH1Strict(t *testing.T) {
	t.Parallel()
	base := report.Report{
		SchemaVersion: 1,
		Score:         95,
		Pages: []report.PageReport{
			pageReport("https://example.com/", result("SINGLE_H1", checks.Warning, "2 h1s", 5)),
		},
	}
	cur := report.Report{
		SchemaVersion: 1,
		Score:         90,
		Pages: []report.PageReport{
			pageReport("https://example.com/", result("SINGLE_H1", checks.Error, "no h1", 10)),
		},
	}
	got := Compare(base, cur, defaultOpts())
	if len(got.Escalated) != 1 || !got.Regressed {
		t.Fatalf("Escalated=%d Regressed=%v", len(got.Escalated), got.Regressed)
	}
}

func TestCompareEscalationMetaAdvisory(t *testing.T) {
	t.Parallel()
	base := report.Report{
		SchemaVersion: 1,
		Score:         97,
		Pages: []report.PageReport{
			pageReport("https://example.com/", result("META_DESCRIPTION", checks.Warning, "short", 3)),
		},
	}
	cur := report.Report{
		SchemaVersion: 1,
		Score:         95,
		Pages: []report.PageReport{
			pageReport("https://example.com/", result("META_DESCRIPTION", checks.Warning, "missing", 5)),
		},
	}
	got := Compare(base, cur, defaultOpts())
	if len(got.Escalated) != 1 {
		t.Fatalf("Escalated = %d, want 1", len(got.Escalated))
	}
	if got.Regressed {
		t.Error("META_DESCRIPTION escalation should be advisory")
	}
}

func TestCompareEscalationImageAltSeverityOnly(t *testing.T) {
	t.Parallel()
	base := report.Report{
		SchemaVersion: 1,
		Score:         95,
		Pages: []report.PageReport{
			pageReport("https://example.com/", result("IMAGE_ALT", checks.Warning, "45%", 5)),
		},
	}
	cur := report.Report{
		SchemaVersion: 1,
		Score:         95,
		Pages: []report.PageReport{
			pageReport("https://example.com/", result("IMAGE_ALT", checks.Error, "50%", 5)),
		},
	}
	got := Compare(base, cur, defaultOpts())
	if len(got.Escalated) != 1 {
		t.Fatalf("Escalated = %d, want 1", len(got.Escalated))
	}
}

func TestCompareDeEscalationIgnored(t *testing.T) {
	t.Parallel()
	base := report.Report{
		SchemaVersion: 1,
		Score:         90,
		Pages: []report.PageReport{
			pageReport("https://example.com/", result("SINGLE_H1", checks.Error, "no h1", 10)),
		},
	}
	cur := report.Report{
		SchemaVersion: 1,
		Score:         95,
		Pages: []report.PageReport{
			pageReport("https://example.com/", result("SINGLE_H1", checks.Warning, "2 h1s", 5)),
		},
	}
	got := Compare(base, cur, defaultOpts())
	if len(got.Escalated) != 0 || len(got.NewFailures) != 0 {
		t.Errorf("de-escalation should not appear in Escalated or NewFailures")
	}
}

func TestCompareViewportSiteWideEscalation(t *testing.T) {
	t.Parallel()
	makePages := func(sev checks.Severity, ded int) []report.PageReport {
		pages := make([]report.PageReport, 10)
		for i := range pages {
			pages[i] = pageReport("https://example.com/p"+string(rune('a'+i)),
				result("VIEWPORT", sev, "viewport issue", ded))
		}
		return pages
	}
	base := report.Report{SchemaVersion: 1, Score: 95, Pages: makePages(checks.Warning, 5)}
	cur := report.Report{SchemaVersion: 1, Score: 90, Pages: makePages(checks.Error, 10)}
	got := Compare(base, cur, defaultOpts())
	if got.ScoreDropExceeded {
		t.Error("Delta == -5 should not exceed max 5")
	}
	if !got.Regressed {
		t.Error("site-wide VIEWPORT escalation should regress via strict rules")
	}
}
