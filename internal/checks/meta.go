package checks

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Erose112/seo-audit/internal/parser"
)

const (
	metaMinLen = 50
	metaMaxLen = 160
)

type MetaDescriptionCheck struct{}

func (MetaDescriptionCheck) ID() string { return "META_DESCRIPTION" }
func (MetaDescriptionCheck) Name() string {
	return "Meta description present and well-sized"
}

// Existence and length share one check because the registry gives the meta
// description a single slot. Missing takes priority over out-of-range so a
// page only trips one failure mode per element.
func (c MetaDescriptionCheck) Run(p parser.PageData) CheckResult {
	desc := strings.TrimSpace(p.MetaDescription)
	if desc == "" {
		return CheckResult{c.ID(), Warning, "Missing meta description", 5}
	}
	n := utf8.RuneCountInString(desc)
	if n < metaMinLen || n > metaMaxLen {
		return CheckResult{
			c.ID(), Warning,
			fmt.Sprintf("Meta description is %d characters (recommended: %d-%d)", n, metaMinLen, metaMaxLen),
			3,
		}
	}
	return CheckResult{c.ID(), Pass, "", 0}
}
