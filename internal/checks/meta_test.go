package checks

import (
	"strings"
	"testing"

	"github.com/Erose112/seo-audit/internal/parser"
)

func TestMetaDescriptionCheck_Boundaries(t *testing.T) {
	cases := []struct {
		name          string
		desc          string
		wantSeverity  Severity
		wantDeduction int
	}{
		{"missing", "", Warning, 5},
		{"whitespace only counts as missing", "   ", Warning, 5},
		{"49 chars - just under range", strings.Repeat("a", 49), Warning, 3},
		{"50 chars - lower bound, in range", strings.Repeat("a", 50), Pass, 0},
		{"160 chars - upper bound, in range", strings.Repeat("a", 160), Pass, 0},
		{"161 chars - just over range", strings.Repeat("a", 161), Warning, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := MetaDescriptionCheck{}.Run(parser.PageData{MetaDescription: tc.desc})
			if result.Severity != tc.wantSeverity {
				t.Errorf("severity = %v, want %v", result.Severity, tc.wantSeverity)
			}
			if result.Deduction != tc.wantDeduction {
				t.Errorf("deduction = %d, want %d", result.Deduction, tc.wantDeduction)
			}
		})
	}
}

func TestMetaDescriptionCheck_Fixture(t *testing.T) {
	result := MetaDescriptionCheck{}.Run(loadFixture(t, "valid_page.html"))
	if result.Severity != Pass {
		t.Errorf("got %v (%q), want pass", result.Severity, result.Message)
	}
}
