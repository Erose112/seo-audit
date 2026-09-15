package regression

import (
	"fmt"
	"io"
	"strings"

	"github.com/Erose112/seo-audit/internal/report"
	"github.com/fatih/color"
)

func (r RegressionResult) WriteText(w io.Writer) error {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)
	bold := color.New(color.Bold)

	if r.Regressed {
		if _, err := bold.Fprintln(w, "SEO REGRESSION DETECTED"); err != nil {
			return err
		}
	} else {
		if _, err := bold.Fprintln(w, "No SEO regression detected."); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Score:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%d → %d (%+d)\n", r.PreviousScore, r.CurrentScore, r.Delta); err != nil {
		return err
	}

	if len(r.NewFailures) > 0 {
		if _, err := fmt.Fprintln(w, "\nNew failures:"); err != nil {
			return err
		}
		for _, f := range r.NewFailures {
			if _, err := red.Fprintf(w, "✗ %s — %s\n", report.PagePath(f.URL), f.Message); err != nil {
				return err
			}
		}
	}

	if len(r.Escalated) > 0 {
		if _, err := fmt.Fprintln(w, "\nEscalated:"); err != nil {
			return err
		}
		for _, e := range r.Escalated {
			fromMsg := e.FromMessage
			if fromMsg == "" {
				fromMsg = string(e.FromSeverity)
			}
			line := fmt.Sprintf("⚠ %s — %s → %s (%s → %s)",
				report.PagePath(e.URL),
				fromMsg,
				e.Message,
				e.FromSeverity,
				e.ToSeverity,
			)
			if _, err := yellow.Fprintln(w, line); err != nil {
				return err
			}
		}
	}

	if len(r.Resolved) > 0 {
		if _, err := fmt.Fprintln(w, "\nResolved:"); err != nil {
			return err
		}
		for _, f := range r.Resolved {
			if _, err := green.Fprintf(w, "✓ %s — %s\n", report.PagePath(f.URL), f.Message); err != nil {
				return err
			}
		}
	}

	if r.Regressed {
		if line := blockingLine(r); line != "" {
			if _, err := fmt.Fprintf(w, "\nBlocking: %s\n", line); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, "\nBuild failed."); err != nil {
			return err
		}
	}

	return nil
}

func blockingLine(r RegressionResult) string {
	var parts []string
	if r.ScoreDropExceeded {
		parts = append(parts, fmt.Sprintf("score dropped %d (max %d)", -r.Delta, r.MaxScoreDrop))
	}
	if len(r.StrictViolations) > 0 {
		ids := uniqueCheckIDs(r.StrictViolations)
		parts = append(parts, "strict rules "+strings.Join(ids, ", "))
	}
	return strings.Join(parts, "; ")
}

func uniqueCheckIDs(violations []Failure) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, f := range violations {
		if _, ok := seen[f.CheckID]; ok {
			continue
		}
		seen[f.CheckID] = struct{}{}
		out = append(out, f.CheckID)
	}
	return out
}
