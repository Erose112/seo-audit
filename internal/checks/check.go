// Package checks runs single-page SEO checks against parsed page data. Each
// check is a pure function over parser.PageData; fetching, scoring and
// reporting live in their own packages.
package checks

import "github.com/Erose112/seo-audit/internal/parser"

type Severity string

const (
	Pass    Severity = "pass"
	Info    Severity = "info"
	Warning Severity = "warning"
	Error   Severity = "error"
)

type CheckResult struct {
	CheckID   string
	Severity  Severity
	Message   string
	Deduction int
}

type Check interface {
	ID() string
	Name() string
	Run(parser.PageData) CheckResult
}

// AllChecks is an explicit registry rather than reflection-based discovery, so
// a missing entry is a visible bug rather than a silently skipped check.
func AllChecks() []Check {
	return []Check{
		TitleExistsCheck{},
		TitleLengthCheck{},
		MetaDescriptionCheck{},
		SingleH1Check{},
		ImageAltCheck{},
		CanonicalCheck{},
		ViewportCheck{},
	}
}
