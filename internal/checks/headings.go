package checks

import (
	"fmt"

	"github.com/Erose112/seo-audit/internal/parser"
)

type SingleH1Check struct{}

func (SingleH1Check) ID() string   { return "SINGLE_H1" }
func (SingleH1Check) Name() string { return "Exactly one H1" }

func (c SingleH1Check) Run(p parser.PageData) CheckResult {
	switch {
	case p.H1Count == 0:
		return CheckResult{c.ID(), Error, "No <h1> found", 10}
	case p.H1Count > 1:
		return CheckResult{
			c.ID(), Warning,
			fmt.Sprintf("%d <h1> elements found; expected exactly 1", p.H1Count),
			5,
		}
	default:
		return CheckResult{c.ID(), Pass, "", 0}
	}
}
