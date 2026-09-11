package checks

import (
	"testing"

	"github.com/Erose112/seo-audit/internal/parser"
)

func TestSingleH1Check_Fixtures(t *testing.T) {
	cases := []struct {
		file string
		want Severity
	}{
		{"valid_page.html", Pass},
		{"multiple_h1.html", Warning},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			result := SingleH1Check{}.Run(loadFixture(t, tc.file))
			if result.Severity != tc.want {
				t.Errorf("got %v, want %v", result.Severity, tc.want)
			}
		})
	}
}

// No zero-H1 fixture exists, so the error path is covered here.
func TestSingleH1Check_Counts(t *testing.T) {
	cases := []struct {
		count         int
		wantSeverity  Severity
		wantDeduction int
	}{
		{0, Error, 10},
		{1, Pass, 0},
		{2, Warning, 5},
		{5, Warning, 5},
	}
	for _, tc := range cases {
		result := SingleH1Check{}.Run(parser.PageData{H1Count: tc.count})
		if result.Severity != tc.wantSeverity {
			t.Errorf("H1Count=%d: severity = %v, want %v", tc.count, result.Severity, tc.wantSeverity)
		}
		if result.Deduction != tc.wantDeduction {
			t.Errorf("H1Count=%d: deduction = %d, want %d", tc.count, result.Deduction, tc.wantDeduction)
		}
	}
}
