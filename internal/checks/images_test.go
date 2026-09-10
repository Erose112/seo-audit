package checks

import (
	"testing"

	"github.com/Erose112/seo-audit/internal/parser"
)

func withAlt(n int) []parser.Image {
	images := make([]parser.Image, n)
	for i := range images {
		images[i] = parser.Image{Src: "img.png", Alt: "ok", HasAlt: true}
	}
	return images
}

func TestImageAltCheck(t *testing.T) {
	cases := []struct {
		name          string
		images        []parser.Image
		wantSeverity  Severity
		wantDeduction int
	}{
		{
			name:          "no images",
			images:        nil,
			wantSeverity:  Pass,
			wantDeduction: 0,
		},
		{
			name: "all alts present, including intentional decorative alt=\"\"",
			images: []parser.Image{
				{Src: "a.png", Alt: "A description", HasAlt: true},
				{Src: "b.png", Alt: "", HasAlt: true}, // decorative, not a failure
			},
			wantSeverity:  Pass,
			wantDeduction: 0,
		},
		{
			name: "one of four missing the attribute entirely",
			images: []parser.Image{
				{Src: "a.png", Alt: "ok", HasAlt: true},
				{Src: "b.png", Alt: "ok", HasAlt: true},
				{Src: "c.png", Alt: "ok", HasAlt: true},
				{Src: "d.png", HasAlt: false},
			},
			wantSeverity:  Warning,
			wantDeduction: 3, // round(10 * 0.25)
		},
		{
			name: "majority missing the attribute",
			images: []parser.Image{
				{Src: "a.png", HasAlt: false},
				{Src: "b.png", HasAlt: false},
				{Src: "c.png", Alt: "ok", HasAlt: true},
			},
			wantSeverity:  Error,
			wantDeduction: 7, // round(10 * 0.667)
		},
		{
			name: "exactly half missing crosses into error",
			images: []parser.Image{
				{Src: "a.png", HasAlt: false},
				{Src: "b.png", Alt: "ok", HasAlt: true},
			},
			wantSeverity:  Error,
			wantDeduction: 5,
		},
		{
			name:          "one of thirty rounds to zero but still deducts",
			images:        append(withAlt(29), parser.Image{Src: "x.png"}),
			wantSeverity:  Warning,
			wantDeduction: 1,
		},
		{
			name:          "all missing",
			images:        []parser.Image{{Src: "a.png"}, {Src: "b.png"}},
			wantSeverity:  Error,
			wantDeduction: 10,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ImageAltCheck{}.Run(parser.PageData{Images: tc.images})
			if result.Severity != tc.wantSeverity {
				t.Errorf("severity = %v, want %v", result.Severity, tc.wantSeverity)
			}
			if result.Deduction != tc.wantDeduction {
				t.Errorf("deduction = %d, want %d", result.Deduction, tc.wantDeduction)
			}
		})
	}
}

// missing_alt.html carries one described image, one decorative alt="" and one
// with no attribute, so only the last should count against the page.
func TestImageAltCheck_Fixtures(t *testing.T) {
	cases := []struct {
		file          string
		wantSeverity  Severity
		wantDeduction int
	}{
		{"valid_page.html", Pass, 0},
		{"missing_alt.html", Warning, 3}, // 1 of 3
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			result := ImageAltCheck{}.Run(loadFixture(t, tc.file))
			if result.Severity != tc.wantSeverity {
				t.Errorf("severity = %v (%q), want %v", result.Severity, result.Message, tc.wantSeverity)
			}
			if result.Deduction != tc.wantDeduction {
				t.Errorf("deduction = %d, want %d", result.Deduction, tc.wantDeduction)
			}
		})
	}
}
