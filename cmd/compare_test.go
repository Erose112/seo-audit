package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/Erose112/seo-audit/internal/regression"
	"github.com/Erose112/seo-audit/internal/report"
	"github.com/spf13/cobra"
)

func TestCompareNoRegressionRunE(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "same.json")
	rep := report.Report{SchemaVersion: report.SchemaVersion, Score: 90}
	if err := regression.SaveBaseline(path, rep); err != nil {
		t.Fatal(err)
	}

	old := compareCfg
	t.Cleanup(func() { compareCfg = old })
	compareCfg.Baseline = path
	compareCfg.Current = path
	compareCfg.MaxScoreDrop = 5
	compareCfg.StrictRules = regression.DefaultStrictRules()

	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := compareCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("compareCmd.RunE: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("No SEO regression detected")) {
		t.Errorf("stdout = %q", stdout.Bytes())
	}
}

func TestCompareStrictViolationRegression(t *testing.T) {
	t.Parallel()
	base := report.Report{
		SchemaVersion: report.SchemaVersion,
		Score:         100,
		Pages:         []report.PageReport{{URL: "https://example.com/", Score: 100}},
	}
	cur := report.Report{
		SchemaVersion: report.SchemaVersion,
		Score:         100,
		SiteIssues: checks.SiteResult{
			DuplicateTitles: map[string][]string{
				"Home": {"https://example.com/", "https://example.com/dup"},
			},
		},
	}
	rr := regression.Compare(base, cur, regression.Options{
		MaxScoreDrop: 5,
		StrictRules:  regression.DefaultStrictRules(),
	})
	if !rr.Regressed {
		t.Fatal("expected duplicate title to regress")
	}
	var buf bytes.Buffer
	if err := rr.WriteText(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("DUPLICATE_TITLE")) {
		t.Errorf("blocking line missing DUPLICATE_TITLE:\n%s", buf.Bytes())
	}
}

func TestCompareAdvisoryFailureDoesNotRegress(t *testing.T) {
	t.Parallel()
	base := report.Report{
		SchemaVersion: report.SchemaVersion,
		Score:         100,
		Pages:         []report.PageReport{{URL: "https://example.com/", Score: 100}},
	}
	cur := report.Report{
		SchemaVersion: report.SchemaVersion,
		Score:         95,
		Pages: []report.PageReport{{
			URL:   "https://example.com/",
			Score: 95,
			Results: []checks.CheckResult{{
				CheckID: "IMAGE_ALT", Severity: checks.Warning, Message: "missing alt", Deduction: 5,
			}},
		}},
	}
	rr := regression.Compare(base, cur, regression.Options{
		MaxScoreDrop: 5,
		StrictRules:  regression.DefaultStrictRules(),
	})
	if rr.Regressed {
		t.Error("IMAGE_ALT alone should be advisory, not regress")
	}
	if len(rr.NewFailures) != 1 {
		t.Errorf("NewFailures = %d, want 1", len(rr.NewFailures))
	}
}

func TestCompareFixtureScoreDrop(t *testing.T) {
	t.Parallel()
	basePath := filepath.Join("..", "testdata", "expected_report.json")
	if _, err := os.Stat(basePath); err != nil {
		t.Skip("expected_report.json not found")
	}
	base, err := regression.LoadReport(basePath)
	if err != nil {
		t.Fatal(err)
	}
	cur := base
	cur.Score = 60
	rr := regression.Compare(base, cur, regression.Options{
		MaxScoreDrop: 5,
		StrictRules:  regression.DefaultStrictRules(),
	})
	if !rr.ScoreDropExceeded {
		t.Error("expected score drop to exceed threshold")
	}
}
