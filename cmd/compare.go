package cmd

import (
	"fmt"
	"os"

	"github.com/Erose112/seo-audit/internal/config"
	"github.com/Erose112/seo-audit/internal/regression"
	"github.com/spf13/cobra"
)

var compareCfg config.CompareConfig

var compareCmd = &cobra.Command{
	Use:          "compare",
	Short:        "Compare a current report against a baseline",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if compareCfg.TextMatchPct != 0 {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning: --text-match-pct not yet implemented; flag ignored")
		}

		strictRules, err := regression.NormalizeRules(compareCfg.StrictRules)
		if err != nil {
			return err
		}

		base, err := regression.LoadReport(compareCfg.Baseline)
		if err != nil {
			return err
		}
		cur, err := regression.LoadReport(compareCfg.Current)
		if err != nil {
			return err
		}

		rr := regression.Compare(base, cur, regression.Options{
			MaxScoreDrop: compareCfg.MaxScoreDrop,
			StrictRules:  strictRules,
		})
		if err := rr.WriteText(cmd.OutOrStdout()); err != nil {
			return err
		}
		if rr.Regressed || cur.Score < compareCfg.FailBelow {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	compareCmd.Flags().StringVar(&compareCfg.Baseline, "baseline", "", "path to baseline JSON report (required)")
	compareCmd.Flags().StringVar(&compareCfg.Current, "current", "", "path to current JSON report (required)")
	compareCmd.Flags().IntVar(&compareCfg.FailBelow, "fail-below", 0, "exit 1 if current score below this")
	compareCmd.Flags().IntVar(&compareCfg.MaxScoreDrop, "max-score-drop", 5, "regression threshold")
	compareCmd.Flags().StringSliceVar(&compareCfg.StrictRules, "strict-rules", regression.DefaultStrictRules(),
		`check IDs whose new failures fail the gate regardless of score ("" to disable)`)
	compareCmd.Flags().IntVar(&compareCfg.TextMatchPct, "text-match-pct", 0, "minimum text-match percentage")
	compareCmd.MarkFlagRequired("baseline")
	compareCmd.MarkFlagRequired("current")
	rootCmd.AddCommand(compareCmd)
}
