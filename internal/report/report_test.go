package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	json "encoding/json/v2"

	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/Erose112/seo-audit/internal/crawler"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/expected_report.json")

func goldenReport() Report {
	ts := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	return Report{
		SchemaVersion: SchemaVersion,
		URL:           "https://example.com/",
		Timestamp:     ts,
		Score:         75,
		PagesCrawled:  2,
		Checks: []CheckSummary{
			{
				CheckID:     "CANONICAL",
				Name:        "Canonical link present",
				Severity:    checks.Pass,
				PagesFailed: 0,
				PagesTotal:  2,
			},
			{
				CheckID:     "IMAGE_ALT",
				Name:        "Images have alt attributes",
				Severity:    checks.Warning,
				PagesFailed: 1,
				PagesTotal:  2,
			},
			{
				CheckID:     "META_DESCRIPTION",
				Name:        "Meta description present and well-sized",
				Severity:    checks.Pass,
				PagesFailed: 0,
				PagesTotal:  2,
			},
			{
				CheckID:     "SINGLE_H1",
				Name:        "Exactly one H1",
				Severity:    checks.Error,
				PagesFailed: 1,
				PagesTotal:  2,
			},
			{
				CheckID:     "TITLE_EXISTS",
				Name:        "Title tag exists",
				Severity:    checks.Pass,
				PagesFailed: 0,
				PagesTotal:  2,
			},
			{
				CheckID:     "TITLE_LENGTH",
				Name:        "Title length within range",
				Severity:    checks.Pass,
				PagesFailed: 0,
				PagesTotal:  2,
			},
			{
				CheckID:     "VIEWPORT",
				Name:        "Responsive viewport meta tag",
				Severity:    checks.Pass,
				PagesFailed: 0,
				PagesTotal:  2,
			},
		},
		Pages: []PageReport{
			{
				URL:   "https://example.com/about",
				Score: 90,
				Results: []checks.CheckResult{
					{CheckID: "TITLE_EXISTS", Severity: checks.Pass, Message: "Title present", Deduction: 0},
					{CheckID: "SINGLE_H1", Severity: checks.Pass, Message: "One H1", Deduction: 0},
					{CheckID: "IMAGE_ALT", Severity: checks.Pass, Message: "All images have alt", Deduction: 0},
				},
			},
			{
				URL:   "https://example.com/home",
				Score: 60,
				Results: []checks.CheckResult{
					{CheckID: "TITLE_EXISTS", Severity: checks.Pass, Message: "Title present", Deduction: 0},
					{CheckID: "SINGLE_H1", Severity: checks.Error, Message: "Multiple H1 tags", Deduction: 10},
					{CheckID: "IMAGE_ALT", Severity: checks.Warning, Message: "Missing alt on 1 image", Deduction: 5},
				},
			},
		},
		SiteIssues: checks.SiteResult{
			DuplicateTitles: map[string][]string{
				"Alpha Title": {
					"https://example.com/a",
					"https://example.com/b",
				},
				"Beta Title": {
					"https://example.com/c",
					"https://example.com/d",
				},
			},
			BrokenLinks: []checks.BrokenLink{
				{
					SourceURL:  "https://example.com/",
					TargetURL:  "https://example.com/missing",
					Kind:       crawler.ErrKindHTTPStatus,
					StatusCode: 404,
					Message:    "404 Not Found",
				},
				{
					SourceURL: "https://example.com/about",
					TargetURL: "https://example.com/gone",
					Kind:      crawler.ErrKindParseFailure,
				},
			},
		},
		Summary: Summary{
			PagesCrawled:    2,
			PagesWithErrors: 1,
			BrokenLinks:     2,
			DuplicateTitles: 2,
			AverageScore:    75,
		},
	}
}

func TestBuildReportSeverityRollup(t *testing.T) {
	in := BuildInput{
		URL:       "https://example.com/",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Crawl: crawler.CrawlResult{
			Pages: make([]crawler.CrawledPage, 2),
		},
		CheckNames: map[string]string{
			"TITLE_EXISTS": "Title tag exists",
			"SINGLE_H1":    "Exactly one H1 tag",
		},
		Pages: []PageReport{
			{
				URL:   "https://example.com/a",
				Score: 90,
				Results: []checks.CheckResult{
					{CheckID: "TITLE_EXISTS", Severity: checks.Pass, Deduction: 0},
					{CheckID: "SINGLE_H1", Severity: checks.Pass, Deduction: 0},
				},
			},
			{
				URL:   "https://example.com/b",
				Score: 80,
				Results: []checks.CheckResult{
					{CheckID: "TITLE_EXISTS", Severity: checks.Warning, Message: "short", Deduction: 5},
					{CheckID: "SINGLE_H1", Severity: checks.Error, Message: "two h1", Deduction: 10},
				},
			},
		},
	}

	got := BuildReport(in)
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, SchemaVersion)
	}
	if got.Score != 85 {
		t.Errorf("Score = %d, want 85", got.Score)
	}
	if got.Summary.AverageScore != 85 {
		t.Errorf("Summary.AverageScore = %d, want 85", got.Summary.AverageScore)
	}

	byID := make(map[string]CheckSummary)
	for _, c := range got.Checks {
		byID[c.CheckID] = c
	}

	title := byID["TITLE_EXISTS"]
	if title.Severity != checks.Warning {
		t.Errorf("TITLE_EXISTS severity = %s, want warning", title.Severity)
	}
	if title.PagesFailed != 1 {
		t.Errorf("TITLE_EXISTS PagesFailed = %d, want 1", title.PagesFailed)
	}
	if title.PagesTotal != 2 {
		t.Errorf("TITLE_EXISTS PagesTotal = %d, want 2", title.PagesTotal)
	}

	h1 := byID["SINGLE_H1"]
	if h1.Severity != checks.Error {
		t.Errorf("SINGLE_H1 severity = %s, want error", h1.Severity)
	}
	if h1.PagesFailed != 1 {
		t.Errorf("SINGLE_H1 PagesFailed = %d, want 1", h1.PagesFailed)
	}
}

func TestWriteJSONEmptyReportUsesArraysNotNull(t *testing.T) {
	var buf bytes.Buffer
	if err := (Report{}).WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "null") {
		t.Errorf("empty report JSON contains null: %s", out)
	}
	for _, want := range []string{`"checks": []`, `"pages": []`, `"broken_links": []`, `"duplicate_titles": {}`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}

func TestGoldenReportJSON(t *testing.T) {
	rep := goldenReport()
	var buf bytes.Buffer
	if err := rep.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	goldenPath := filepath.Join("..", "..", "testdata", "expected_report.json")
	if *updateGolden {
		if err := os.WriteFile(goldenPath, buf.Bytes(), 0644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", buf.Bytes(), want)
	}

	var decoded Report
	if err := json.UnmarshalRead(bytes.NewReader(want), &decoded); err != nil {
		t.Fatalf("UnmarshalRead golden: %v", err)
	}
	if decoded.SchemaVersion != SchemaVersion {
		t.Errorf("decoded SchemaVersion = %d, want %d", decoded.SchemaVersion, SchemaVersion)
	}
	if len(decoded.SiteIssues.BrokenLinks) != 2 {
		t.Errorf("decoded broken links = %d, want 2", len(decoded.SiteIssues.BrokenLinks))
	}
}
