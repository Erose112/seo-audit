package cmd

import (
	"fmt"

	"github.com/Erose112/seo-audit/internal/config"
	"github.com/spf13/cobra"
)

var compareCfg config.CompareConfig

var compareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare a current report against a baseline",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("%+v\n", compareCfg)
		return nil
	},
}

func init() {
	compareCmd.Flags().StringVar(&compareCfg.Baseline, "baseline", "", "path to baseline JSON report (required)")
	compareCmd.Flags().StringVar(&compareCfg.Current, "current", "", "path to current JSON report (required)")
	compareCmd.Flags().IntVar(&compareCfg.FailBelow, "fail-below", 0, "exit 1 if current score below this")
	compareCmd.Flags().IntVar(&compareCfg.MaxScoreDrop, "max-score-drop", 5, "regression threshold")
	compareCmd.Flags().StringSliceVar(&compareCfg.StrictRules, "strict-rules", nil, "check IDs that must not newly fail")
	compareCmd.Flags().IntVar(&compareCfg.TextMatchPct, "text-match-pct", 0, "minimum text-match percentage")
	compareCmd.MarkFlagRequired("baseline")
	compareCmd.MarkFlagRequired("current")
	rootCmd.AddCommand(compareCmd)
}
