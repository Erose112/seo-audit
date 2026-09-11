package checks

import "github.com/Erose112/seo-audit/internal/parser"

type CanonicalCheck struct{}

func (CanonicalCheck) ID() string   { return "CANONICAL" }
func (CanonicalCheck) Name() string { return "Canonical link present" }

// Existence only. Whether the canonical is self-referential or same-domain is
// a cross-page question, not a v1 single-page one.
func (c CanonicalCheck) Run(p parser.PageData) CheckResult {
	if p.Canonical == "" {
		return CheckResult{c.ID(), Warning, `Missing <link rel="canonical">`, 5}
	}
	return CheckResult{c.ID(), Pass, "", 0}
}
