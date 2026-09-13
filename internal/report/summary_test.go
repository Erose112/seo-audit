package report

import (
	"testing"

	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/Erose112/seo-audit/internal/crawler"
)

func TestBuildSummary(t *testing.T) {
	cr := crawler.CrawlResult{
		Pages: make([]crawler.CrawledPage, 4),
		Errors: []crawler.CrawlError{
			{URL: "https://example.com/404", Kind: crawler.ErrKindHTTPStatus, StatusCode: 404},
			{URL: "https://example.com/json", Kind: crawler.ErrKindParseFailure, Message: "not HTML"},
		},
	}
	sr := checks.SiteResult{
		DuplicateTitles: map[string][]string{
			"Dup": {"https://example.com/a", "https://example.com/b"},
		},
		BrokenLinks: make([]checks.BrokenLink, 3),
	}
	pageScores := []int{90, 80, 70, 60}

	got := BuildSummary(cr, sr, pageScores)

	if got.PagesCrawled != 4 {
		t.Errorf("PagesCrawled = %d, want 4", got.PagesCrawled)
	}
	if got.PagesWithErrors != 2 {
		t.Errorf("PagesWithErrors = %d, want 2 (crawl errors including parse failure)", got.PagesWithErrors)
	}
	if got.BrokenLinks != 3 {
		t.Errorf("BrokenLinks = %d, want 3", got.BrokenLinks)
	}
	if got.DuplicateTitles != 1 {
		t.Errorf("DuplicateTitles = %d, want 1", got.DuplicateTitles)
	}
	if got.AverageScore != 75 {
		t.Errorf("AverageScore = %d, want 75 (integer division of 300/4)", got.AverageScore)
	}
}

func TestBuildSummaryEmptyScores(t *testing.T) {
	got := BuildSummary(crawler.CrawlResult{}, checks.SiteResult{}, nil)
	if got.AverageScore != 0 {
		t.Errorf("AverageScore = %d, want 0 for empty pageScores", got.AverageScore)
	}
}
