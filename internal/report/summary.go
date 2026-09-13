package report

import (
	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/Erose112/seo-audit/internal/crawler"
	"github.com/Erose112/seo-audit/internal/scoring"
)

type Summary struct {
	PagesCrawled    int
	PagesWithErrors int
	BrokenLinks     int
	DuplicateTitles int
	AverageScore    int
}

// BuildSummary aggregates crawl, site-wide, and scoring inputs into counts.
// PagesWithErrors is the number of crawl/fetch failures (including parse
// failures), not pages with check-severity errors.
func BuildSummary(cr crawler.CrawlResult, sr checks.SiteResult, pageScores []int) Summary {
	return Summary{
		PagesCrawled:    len(cr.Pages),
		PagesWithErrors: len(cr.Errors),
		BrokenLinks:     len(sr.BrokenLinks),
		DuplicateTitles: len(sr.DuplicateTitles),
		AverageScore:    scoring.ScoreSite(pageScores),
	}
}
