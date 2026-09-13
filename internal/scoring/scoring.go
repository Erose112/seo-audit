// Package scoring turns check results into a 0-100 score. It is deterministic.
package scoring

import "github.com/Erose112/seo-audit/internal/checks"

// Weights names the per-severity deduction budget the Stage 4 checks were
// written against. It is the single documented home for those numbers; the
// checks still carry their own deduction because severity alone can't express
// magnitude (IMAGE_ALT scales its deduction with the share of images missing
// alt text, and META_DESCRIPTION costs 5 when absent but 3 when badly sized).
type Weights struct {
	Error   int
	Warning int
	Info    int
}

func DefaultWeights() Weights { return Weights{Error: 10, Warning: 5, Info: 0} }

// ScorePage subtracts every result's deduction from 100 and clamps to 0-100.
//
// w is accepted but deliberately unused in v1, so the signature won't have to
// change when weighting goes live. Deductions belong to the checks (Stage 4),
// and summing them verbatim keeps the score reconstructable from the JSON
// report, which serializes CheckResult.Deduction.
//
// TODO(stage-12): make w actually configurable once thresholds are tuned
// against real pages. Rescale rather than replace — scale each deduction by
// the configured weight over the default weight for its severity — so the
// defaults stay a no-op and per-check magnitudes survive. Replacing
// deductions with a flat per-severity lookup would flatten IMAGE_ALT's
// proportional curve and make its 50% Warning/Error line a score cliff.
// TestScorePage_WeightsCurrentlyDoNotAffectScore guards the current behavior
// and is expected to be updated by that change.
func ScorePage(results []checks.CheckResult, w Weights) int {
	score := 100
	for _, r := range results {
		score -= r.Deduction
	}
	return clamp(score, 0, 100)
}

// ScoreSite averages page scores with integer division, so the result is
// truncated rather than rounded.
func ScoreSite(pageScores []int) int {
	if len(pageScores) == 0 {
		return 0
	}
	sum := 0
	for _, s := range pageScores {
		sum += s
	}
	return sum / len(pageScores)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
