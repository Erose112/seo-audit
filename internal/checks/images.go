package checks

import (
	"fmt"
	"math"

	"github.com/Erose112/seo-audit/internal/parser"
)

type ImageAltCheck struct{}

func (ImageAltCheck) ID() string   { return "IMAGE_ALT" }
func (ImageAltCheck) Name() string { return "Images have alt attributes" }

// Missing means the alt attribute is absent. alt="" is the decorative-image
// pattern that tells screen readers to skip the image, so it passes.
func (c ImageAltCheck) Run(p parser.PageData) CheckResult {
	total := len(p.Images)
	if total == 0 {
		return CheckResult{c.ID(), Pass, "No images found", 0}
	}

	missing := 0
	for _, img := range p.Images {
		if !img.HasAlt {
			missing++
		}
	}
	if missing == 0 {
		return CheckResult{c.ID(), Pass, "", 0}
	}

	// Proportional rather than a flat count threshold, so 1-of-20 and 18-of-20
	// don't score the same.
	pct := float64(missing) / float64(total)
	deduction := int(math.Round(10 * pct))
	// On a page with enough images the rounded share can land on zero; a
	// flagged failure must still cost something for Stage 5 to see it.
	if deduction == 0 {
		deduction = 1
	}
	severity := Warning
	if pct >= 0.5 {
		severity = Error
	}
	message := fmt.Sprintf("%d of %d images missing alt attribute (%.0f%%)", missing, total, pct*100)
	return CheckResult{c.ID(), severity, message, deduction}
}
