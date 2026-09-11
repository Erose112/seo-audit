package checks

import (
	"testing"

	"github.com/Erose112/seo-audit/internal/parser"
)

func TestCanonicalCheck_Fixtures(t *testing.T) {
	cases := []struct {
		file string
		want Severity
	}{
		{"valid_page.html", Pass},
		{"missing_canonical.html", Warning},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			result := CanonicalCheck{}.Run(loadFixture(t, tc.file))
			if result.Severity != tc.want {
				t.Errorf("got %v, want %v", result.Severity, tc.want)
			}
		})
	}
}

func TestCanonicalCheck_MissingDeducts(t *testing.T) {
	result := CanonicalCheck{}.Run(parser.PageData{})
	if result.Severity != Warning || result.Deduction != 5 {
		t.Errorf("got %v/%d, want warning/5", result.Severity, result.Deduction)
	}
}
