package checks

import (
	"fmt"
	"strings"

	"github.com/Erose112/seo-audit/internal/parser"
)

type ViewportCheck struct{}

func (ViewportCheck) ID() string   { return "VIEWPORT" }
func (ViewportCheck) Name() string { return "Responsive viewport meta tag" }

func (c ViewportCheck) Run(p parser.PageData) CheckResult {
	content := strings.TrimSpace(p.Viewport)
	if content == "" {
		return CheckResult{c.ID(), Error, `Missing <meta name="viewport"> tag`, 10}
	}
	// Real markup varies in spacing, casing and directive order, e.g.
	// "width = device-width" or "initial-scale=1, width=device-width".
	normalized := strings.ToLower(strings.ReplaceAll(content, " ", ""))
	if !strings.Contains(normalized, "width=device-width") {
		return CheckResult{
			c.ID(), Warning,
			fmt.Sprintf("Viewport tag present but missing width=device-width (got %q)", content),
			5,
		}
	}
	return CheckResult{c.ID(), Pass, "", 0}
}
