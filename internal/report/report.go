package report

import (
	"io"
	"sort"
	"time"

	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/Erose112/seo-audit/internal/crawler"
	"github.com/Erose112/seo-audit/internal/scoring"
)

// SchemaVersion is the CI-facing contract version documented in
// docs/report-schema.md. Bump it only for a breaking change.
const SchemaVersion = 1

type Report struct {
	SchemaVersion int               `json:"schema_version"`
	URL           string            `json:"url"`
	Timestamp     time.Time         `json:"timestamp"`
	Score         int               `json:"score"`
	PagesCrawled  int               `json:"pages_crawled"`
	Checks        []CheckSummary    `json:"checks"`
	Pages         []PageReport      `json:"pages"`
	SiteIssues    checks.SiteResult `json:"site_issues"`
	Summary       Summary           `json:"summary"`
}

type PageReport struct {
	URL     string               `json:"url"`
	Score   int                  `json:"score"`
	Results []checks.CheckResult `json:"results"`
}

// CheckSummary is the site-level roll-up the text report's check list is built from.
type CheckSummary struct {
	CheckID     string          `json:"check_id"`
	Name        string          `json:"name"`
	Severity    checks.Severity `json:"severity"` // worst severity seen across pages
	PagesFailed int             `json:"pages_failed"`
	PagesTotal  int             `json:"pages_total"`
}

// BuildInput replaces the doc's positional BuildReport args now that the
// pipeline, not the report package, runs the checks.
type BuildInput struct {
	URL        string
	Timestamp  time.Time // passed in, not time.Now(), so tests are deterministic
	Crawl      crawler.CrawlResult
	Site       checks.SiteResult
	Pages      []PageReport
	CheckNames map[string]string // CheckID -> Name, from the checks.AllChecks registry
}

type checkRollup struct {
	checkID     string
	name        string
	severity    checks.Severity
	pagesFailed int
	pagesTotal  int
}

func severityRank(s checks.Severity) int {
	switch s {
	case checks.Error:
		return 4
	case checks.Warning:
		return 3
	case checks.Info:
		return 2
	case checks.Pass:
		return 1
	default:
		return 0
	}
}

func isFailedSeverity(s checks.Severity) bool {
	return s == checks.Warning || s == checks.Error
}

func worseSeverity(a, b checks.Severity) checks.Severity {
	if severityRank(a) >= severityRank(b) {
		return a
	}
	return b
}

// BuildReport assembles the final report from precomputed page results and
// site-wide findings.
func BuildReport(in BuildInput) Report {
	pageScores := make([]int, len(in.Pages))
	for i, p := range in.Pages {
		pageScores[i] = p.Score
	}

	rollupByID := make(map[string]*checkRollup)
	for _, page := range in.Pages {
		for _, result := range page.Results {
			entry, ok := rollupByID[result.CheckID]
			if !ok {
				entry = &checkRollup{
					checkID:  result.CheckID,
					name:     in.CheckNames[result.CheckID],
					severity: checks.Pass,
				}
				rollupByID[result.CheckID] = entry
			}
			entry.pagesTotal++
			entry.severity = worseSeverity(entry.severity, result.Severity)
			if isFailedSeverity(result.Severity) {
				entry.pagesFailed++
			}
		}
	}

	checkSummaries := make([]CheckSummary, 0, len(rollupByID))
	for _, entry := range rollupByID {
		checkSummaries = append(checkSummaries, CheckSummary{
			CheckID:     entry.checkID,
			Name:        entry.name,
			Severity:    entry.severity,
			PagesFailed: entry.pagesFailed,
			PagesTotal:  entry.pagesTotal,
		})
	}
	sort.Slice(checkSummaries, func(i, j int) bool {
		return checkSummaries[i].CheckID < checkSummaries[j].CheckID
	})

	pages := in.Pages
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].URL < pages[j].URL
	})

	score := scoring.ScoreSite(pageScores)

	return Report{
		SchemaVersion: SchemaVersion,
		URL:           in.URL,
		Timestamp:     in.Timestamp,
		Score:         score,
		PagesCrawled:  len(in.Crawl.Pages),
		Checks:        checkSummaries,
		Pages:         pages,
		SiteIssues:    in.Site,
		Summary:       BuildSummary(in.Crawl, in.Site, pageScores),
	}
}

func (r Report) WriteJSON(w io.Writer) error {
	if err := json.MarshalWrite(w, r,
		jsontext.WithIndent("  "),
		json.Deterministic(true),
	); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}
