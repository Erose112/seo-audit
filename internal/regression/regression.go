package regression

import (
	"fmt"
	"sort"

	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/Erose112/seo-audit/internal/report"
)

const (
	CheckIDDuplicateTitle = "DUPLICATE_TITLE"
	CheckIDBrokenLink     = "BROKEN_LINK"
)

type RegressionResult struct {
	PreviousScore     int
	CurrentScore      int
	Delta             int
	NewFailures       []Failure
	Resolved          []Failure
	Escalated         []Escalation
	StrictViolations  []Failure
	ScoreDropExceeded bool
	MaxScoreDrop      int // threshold used; for text output only
	Regressed         bool
}

type Failure struct {
	URL     string
	CheckID string
	Message string
}

type Escalation struct {
	Failure
	FromMessage   string
	FromSeverity  checks.Severity
	ToSeverity    checks.Severity
	FromDeduction int
	ToDeduction   int
}

type Options struct {
	MaxScoreDrop int
	StrictRules  []string // already normalized; see NormalizeRules
}

// DefaultStrictRules are the template-level checks that essentially never fail
// on purpose, so a new failure in any of them fails the gate outright. The
// author-level checks (TITLE_LENGTH, META_DESCRIPTION, IMAGE_ALT) are left to
// the score, since routine content edits trip them constantly.
func DefaultStrictRules() []string {
	return []string{
		"TITLE_EXISTS",
		"SINGLE_H1",
		"VIEWPORT",
		"CANONICAL",
		CheckIDBrokenLink,
		CheckIDDuplicateTitle,
	}
}

type failureKey struct {
	URL, CheckID, Detail string
}

type finding struct {
	Failure
	Severity  checks.Severity
	Deduction int
}

func isFailing(s checks.Severity) bool {
	return s == checks.Warning || s == checks.Error
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

func isEscalated(base, cur finding) bool {
	return severityRank(cur.Severity) > severityRank(base.Severity) || cur.Deduction > base.Deduction
}

func failingSet(r report.Report) map[failureKey]finding {
	out := make(map[failureKey]finding)

	for _, page := range r.Pages {
		for _, result := range page.Results {
			if !isFailing(result.Severity) {
				continue
			}
			key := failureKey{URL: page.URL, CheckID: result.CheckID}
			out[key] = finding{
				Failure: Failure{
					URL:     page.URL,
					CheckID: result.CheckID,
					Message: result.Message,
				},
				Severity:  result.Severity,
				Deduction: result.Deduction,
			}
		}
	}

	for title, urls := range r.SiteIssues.DuplicateTitles {
		for _, u := range urls {
			key := failureKey{URL: u, CheckID: CheckIDDuplicateTitle}
			out[key] = finding{
				Failure: Failure{
					URL:     u,
					CheckID: CheckIDDuplicateTitle,
					Message: fmt.Sprintf("duplicate title %q", title),
				},
				Severity:  checks.Error,
				Deduction: 0,
			}
		}
	}

	for _, bl := range r.SiteIssues.BrokenLinks {
		key := failureKey{
			URL:     bl.SourceURL,
			CheckID: CheckIDBrokenLink,
			Detail:  bl.TargetURL,
		}
		msg := fmt.Sprintf("broken link to %s (%s)", bl.TargetURL, bl.Kind)
		if bl.StatusCode != 0 {
			msg = fmt.Sprintf("broken link to %s (%s %d)", bl.TargetURL, bl.Kind, bl.StatusCode)
		}
		if bl.Message != "" {
			msg += ": " + bl.Message
		}
		out[key] = finding{
			Failure: Failure{
				URL:     bl.SourceURL,
				CheckID: CheckIDBrokenLink,
				Message: msg,
			},
			Severity:  checks.Error,
			Deduction: 0,
		}
	}

	return out
}

func currentURLs(pages []report.PageReport) map[string]struct{} {
	out := make(map[string]struct{}, len(pages))
	for _, p := range pages {
		out[p.URL] = struct{}{}
	}
	return out
}

func strictViolations(newFailures []Failure, escalated []Escalation, strictRules []string) []Failure {
	if len(strictRules) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(strictRules))
	for _, id := range strictRules {
		allowed[id] = struct{}{}
	}

	var out []Failure
	for _, f := range newFailures {
		if _, ok := allowed[f.CheckID]; ok {
			out = append(out, f)
		}
	}
	for _, e := range escalated {
		if _, ok := allowed[e.CheckID]; ok {
			out = append(out, e.Failure)
		}
	}
	sortFailures(out)
	return out
}

func sortFailures(f []Failure) {
	sort.Slice(f, func(i, j int) bool {
		if f[i].URL != f[j].URL {
			return f[i].URL < f[j].URL
		}
		return f[i].CheckID < f[j].CheckID
	})
}

func sortEscalations(e []Escalation) {
	sort.Slice(e, func(i, j int) bool {
		if e[i].URL != e[j].URL {
			return e[i].URL < e[j].URL
		}
		if e[i].CheckID != e[j].CheckID {
			return e[i].CheckID < e[j].CheckID
		}
		return e[i].Message < e[j].Message
	})
}

// Compare diffs two reports keyed by failing findings. baseline.SchemaVersion
// == 0 means no prior baseline (first run); comparison is skipped.
func Compare(baseline, current report.Report, opts Options) RegressionResult {
	result := RegressionResult{
		PreviousScore: baseline.Score,
		CurrentScore:  current.Score,
		Delta:         current.Score - baseline.Score,
	}

	if baseline.SchemaVersion == 0 {
		return result
	}

	baseSet := failingSet(baseline)
	curSet := failingSet(current)
	present := currentURLs(current.Pages)

	for key, cur := range curSet {
		base, had := baseSet[key]
		switch {
		case !had:
			result.NewFailures = append(result.NewFailures, cur.Failure)
		case isEscalated(base, cur):
			result.Escalated = append(result.Escalated, Escalation{
				Failure:       cur.Failure,
				FromMessage:   base.Message,
				FromSeverity:  base.Severity,
				ToSeverity:    cur.Severity,
				FromDeduction: base.Deduction,
				ToDeduction:   cur.Deduction,
			})
		}
	}

	for key, base := range baseSet {
		if _, still := curSet[key]; still {
			continue
		}
		if _, onSite := present[base.URL]; !onSite {
			continue
		}
		result.Resolved = append(result.Resolved, base.Failure)
	}

	sortFailures(result.NewFailures)
	sortFailures(result.Resolved)
	sortEscalations(result.Escalated)

	result.MaxScoreDrop = opts.MaxScoreDrop
	result.ScoreDropExceeded = result.Delta < -opts.MaxScoreDrop
	result.StrictViolations = strictViolations(result.NewFailures, result.Escalated, opts.StrictRules)
	result.Regressed = result.ScoreDropExceeded || len(result.StrictViolations) > 0

	return result
}
