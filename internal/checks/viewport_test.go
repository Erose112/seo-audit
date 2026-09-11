package checks

import (
	"testing"

	"github.com/Erose112/seo-audit/internal/parser"
)

func TestViewportCheck_Fixtures(t *testing.T) {
	cases := []struct {
		file string
		want Severity
	}{
		{"valid_page.html", Pass},
		{"no_viewport.html", Error},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			result := ViewportCheck{}.Run(loadFixture(t, tc.file))
			if result.Severity != tc.want {
				t.Errorf("got %v, want %v", result.Severity, tc.want)
			}
		})
	}
}

func TestViewportCheck_ContentVariants(t *testing.T) {
	cases := []struct {
		name          string
		content       string
		wantSeverity  Severity
		wantDeduction int
	}{
		{"absent", "", Error, 10},
		{"whitespace only counts as absent", "   ", Error, 10},
		{"canonical form", "width=device-width, initial-scale=1", Pass, 0},
		{"spaces around the equals sign", "width = device-width", Pass, 0},
		{"uppercase", "WIDTH=DEVICE-WIDTH", Pass, 0},
		{"directive order reversed", "initial-scale=1, width=device-width", Pass, 0},
		{"present but fixed width", "width=1024", Warning, 5},
		{"present but scale only", "initial-scale=1", Warning, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ViewportCheck{}.Run(parser.PageData{Viewport: tc.content})
			if result.Severity != tc.wantSeverity {
				t.Errorf("severity = %v (%q), want %v", result.Severity, result.Message, tc.wantSeverity)
			}
			if result.Deduction != tc.wantDeduction {
				t.Errorf("deduction = %d, want %d", result.Deduction, tc.wantDeduction)
			}
		})
	}
}
