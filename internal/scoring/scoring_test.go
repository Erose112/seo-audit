package scoring

import (
	"testing"

	"github.com/Erose112/seo-audit/internal/checks"
)

func result(id string, sev checks.Severity, deduction int) checks.CheckResult {
	return checks.CheckResult{CheckID: id, Severity: sev, Deduction: deduction}
}

func TestScorePage(t *testing.T) {
	cases := []struct {
		name    string
		results []checks.CheckResult
		want    int
	}{
		{
			name:    "no results scores 100",
			results: nil,
			want:    100,
		},
		{
			name: "all passing scores 100",
			results: []checks.CheckResult{
				result("TITLE_EXISTS", checks.Pass, 0),
				result("CANONICAL", checks.Pass, 0),
			},
			want: 100,
		},
		{
			name: "deductions sum",
			results: []checks.CheckResult{
				result("TITLE_EXISTS", checks.Error, 10),
				result("CANONICAL", checks.Warning, 5),
				result("META_DESCRIPTION", checks.Warning, 3),
			},
			want: 82,
		},
		{
			name: "same severity can deduct different amounts",
			results: []checks.CheckResult{
				result("IMAGE_ALT", checks.Warning, 1),
				result("META_DESCRIPTION", checks.Warning, 5),
			},
			want: 94,
		},
		{
			name: "worst case across the Stage 4 registry",
			results: []checks.CheckResult{
				result("TITLE_EXISTS", checks.Error, 10),
				result("TITLE_LENGTH", checks.Warning, 5),
				result("META_DESCRIPTION", checks.Warning, 5),
				result("SINGLE_H1", checks.Error, 10),
				result("IMAGE_ALT", checks.Error, 10),
				result("CANONICAL", checks.Warning, 5),
				result("VIEWPORT", checks.Error, 10),
			},
			want: 45,
		},
		{
			name: "clamps to zero instead of going negative",
			results: []checks.CheckResult{
				result("A", checks.Error, 60),
				result("B", checks.Error, 60),
			},
			want: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ScorePage(tc.results, DefaultWeights()); got != tc.want {
				t.Errorf("ScorePage() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestScorePageIsDeterministic(t *testing.T) {
	results := []checks.CheckResult{
		result("TITLE_EXISTS", checks.Error, 10),
		result("IMAGE_ALT", checks.Warning, 3),
	}
	first := ScorePage(results, DefaultWeights())
	for i := 0; i < 100; i++ {
		if got := ScorePage(results, DefaultWeights()); got != first {
			t.Fatalf("run %d returned %d, first run returned %d", i, got, first)
		}
	}
}

// Characterization test, not an aspiration: in v1 ScorePage sums
// CheckResult.Deduction and ignores w entirely. Weights exists so the
// signature is stable when weighting goes live (see the TODO(stage-12) on
// ScorePage). If you are here because this test failed, that is the point —
// weighting presumably just became real, so replace this test with one
// asserting the new behavior and say so in the commit message.
func TestScorePage_WeightsCurrentlyDoNotAffectScore(t *testing.T) {
	results := []checks.CheckResult{result("TITLE_EXISTS", checks.Error, 10)}

	got := ScorePage(results, DefaultWeights())
	absurd := ScorePage(results, Weights{Error: 999, Warning: 999, Info: 999})
	zeroed := ScorePage(results, Weights{})

	if got != 90 {
		t.Errorf("default weights: got %d, want 90 (100 - the check's own deduction)", got)
	}
	if absurd != got {
		t.Errorf("inflated weights changed the score: %d vs %d", absurd, got)
	}
	if zeroed != got {
		t.Errorf("zero-value weights changed the score: %d vs %d", zeroed, got)
	}
}

func TestScoreSite(t *testing.T) {
	cases := []struct {
		name       string
		pageScores []int
		want       int
	}{
		{
			name:       "no pages scores 0",
			pageScores: nil,
			want:       0,
		},
		{
			name:       "single page is its own score",
			pageScores: []int{87},
			want:       87,
		},
		{
			name:       "exact average",
			pageScores: []int{90, 80},
			want:       85,
		},
		{
			name:       "truncates rather than rounds",
			pageScores: []int{100, 100, 89},
			want:       96, // 289/3 = 96.33
		},
		{
			name:       "truncates down even when rounding would go up",
			pageScores: []int{100, 100, 99},
			want:       99, // 299/3 = 99.67
		},
		{
			name:       "a single zero-scoring page drags the average",
			pageScores: []int{100, 0},
			want:       50,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ScoreSite(tc.pageScores); got != tc.want {
				t.Errorf("ScoreSite() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestScoreSiteIgnoresOrder(t *testing.T) {
	ascending := ScoreSite([]int{40, 70, 100})
	descending := ScoreSite([]int{100, 70, 40})
	if ascending != descending {
		t.Errorf("order changed the site score: %d vs %d", ascending, descending)
	}
}
