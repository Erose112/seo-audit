package checks

import (
	"strings"
	"testing"

	"github.com/Erose112/seo-audit/internal/parser"
)

func TestTitleExistsCheck_Fixtures(t *testing.T) {
	cases := []struct {
		file string
		want Severity
	}{
		{"valid_page.html", Pass},
		{"missing_title.html", Error},
		{"malformed.html", Pass}, // lenient parsing still recovers the title
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			result := TitleExistsCheck{}.Run(loadFixture(t, tc.file))
			if result.Severity != tc.want {
				t.Errorf("got %v, want %v", result.Severity, tc.want)
			}
		})
	}
}

func TestTitleExistsCheck_WhitespaceOnly(t *testing.T) {
	result := TitleExistsCheck{}.Run(parser.PageData{Title: "   "})
	if result.Severity != Error || result.Deduction != 10 {
		t.Errorf("got %v/%d, want error/10", result.Severity, result.Deduction)
	}
}

func TestTitleLengthCheck_Boundaries(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  Severity
	}{
		{"empty title defers to TITLE_EXISTS", "", Pass},
		{"29 chars - just under range", strings.Repeat("a", 29), Warning},
		{"30 chars - lower bound, in range", strings.Repeat("a", 30), Pass},
		{"60 chars - upper bound, in range", strings.Repeat("a", 60), Pass},
		{"61 chars - just over range", strings.Repeat("a", 61), Warning},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := TitleLengthCheck{}.Run(parser.PageData{Title: tc.title})
			if result.Severity != tc.want {
				t.Errorf("got %v, want %v", result.Severity, tc.want)
			}
		})
	}
}

// Multi-byte titles must be measured in characters, not bytes: 40 CJK runes is
// in range even though len() would report 120.
func TestTitleLengthCheck_CountsRunesNotBytes(t *testing.T) {
	title := strings.Repeat("漢", 40)
	result := TitleLengthCheck{}.Run(parser.PageData{Title: title})
	if result.Severity != Pass {
		t.Errorf("got %v (%q), want pass", result.Severity, result.Message)
	}
}
