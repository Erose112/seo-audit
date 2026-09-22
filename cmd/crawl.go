package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/Erose112/seo-audit/internal/config"
	"github.com/Erose112/seo-audit/internal/crawler"
	"github.com/Erose112/seo-audit/internal/regression"
	"github.com/Erose112/seo-audit/internal/report"
	"github.com/Erose112/seo-audit/internal/scoring"
	"github.com/spf13/cobra"
)

var crawlCfg config.CrawlConfig

var crawlCmd = &cobra.Command{
	Use:          "crawl",
	Short:        "Crawl a site and produce an SEO audit report",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch crawlCfg.Output {
		case "json", "text":
		default:
			return fmt.Errorf("invalid --output %q: want text or json", crawlCfg.Output)
		}

		strictRules, err := regression.NormalizeRules(crawlCfg.StrictRules)
		if err != nil {
			return err
		}

		var baselineRep report.Report
		if crawlCfg.Baseline != "" {
			baselineRep, err = regression.LoadBaseline(crawlCfg.Baseline)
			if err != nil {
				return err
			}
		}

		ctx, cancel := crawler.NewCrawlContext(crawlCfg.MaxDuration)
		defer cancel()

		rep, runErr := runCrawl(ctx, crawlCfg)
		if runErr != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "crawl error:", runErr)
			os.Exit(2)
		}

		switch crawlCfg.Output {
		case "json":
			if err := rep.WriteJSON(cmd.OutOrStdout()); err != nil {
				return err
			}
		case "text":
			if err := rep.WriteText(cmd.OutOrStdout()); err != nil {
				return err
			}
		}

		var regressed bool
		if crawlCfg.Baseline != "" {
			rr := regression.Compare(baselineRep, rep, regression.Options{
				MaxScoreDrop: crawlCfg.MaxScoreDrop,
				StrictRules:  strictRules,
			})
			regressed = rr.Regressed
			if regressed {
				if err := rr.WriteText(cmd.ErrOrStderr()); err != nil {
					return err
				}
			}
			if err := regression.SaveBaseline(crawlCfg.Baseline, rep); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: failed to save baseline:", err)
			}
		}

		if code := exitCodeFor(rep, crawlCfg, nil, regressed); code != 0 {
			os.Exit(code)
		}
		return nil
	},
}

// exitCodeFor maps crawl outcome to the stable CI exit-code contract:
// 0 = pass, 1 = score gate failure or regression, 2 = systemic/CLI error.
func exitCodeFor(rep report.Report, cfg config.CrawlConfig, runErr error, regressed bool) int {
	if runErr != nil {
		return 2
	}
	if regressed || rep.Score < cfg.FailBelow {
		return 1
	}
	return 0
}

func runCrawl(ctx context.Context, cfg config.CrawlConfig) (report.Report, error) {
	crawlerCfg := crawler.DefaultConfig()
	if cfg.MaxBodySize > 0 {
		crawlerCfg.MaxBodySize = cfg.MaxBodySize
	}

	fetcher, err := crawler.NewFetcher(crawler.NewClient(), crawlerCfg)
	if err != nil {
		return report.Report{}, err
	}

	result, err := crawler.Crawl(ctx, fetcher, cfg.URL, crawler.CrawlOptions{
		MaxPages: cfg.MaxPages,
		MaxDepth: cfg.MaxDepth,
		Delay:    cfg.Delay,
	})
	if err != nil {
		return report.Report{}, err
	}

	checkNames := make(map[string]string)
	for _, c := range checks.AllChecks() {
		checkNames[c.ID()] = c.Name()
	}

	weights := scoring.DefaultWeights()
	pages := make([]report.PageReport, 0, len(result.Pages))

	for _, p := range result.Pages {
		results := make([]checks.CheckResult, 0, len(checks.AllChecks()))
		for _, c := range checks.AllChecks() {
			results = append(results, c.Run(p.Data))
		}
		score := scoring.ScorePage(results, weights)
		pages = append(pages, report.PageReport{
			URL:     p.URL,
			Score:   score,
			Results: results,
		})
	}

	site := checks.SiteResult{
		DuplicateTitles: checks.FindDuplicateTitles(result.Pages),
		BrokenLinks:     checks.FindBrokenLinks(result.Pages, result.Errors),
	}

	return report.BuildReport(report.BuildInput{
		URL:        cfg.URL,
		Timestamp:  time.Now().UTC(),
		Crawl:      result,
		Site:       site,
		Pages:      pages,
		CheckNames: checkNames,
	}), nil
}

func init() {
	crawlCmd.Flags().StringVar(&crawlCfg.URL, "url", "", "target URL (required)")
	crawlCmd.Flags().IntVar(&crawlCfg.MaxPages, "max-pages", 100, "max pages to crawl")
	crawlCmd.Flags().IntVar(&crawlCfg.MaxDepth, "max-depth", 5, "max crawl depth")
	crawlCmd.Flags().DurationVar(&crawlCfg.Delay, "delay", 200*time.Millisecond, "delay between requests")
	crawlCmd.Flags().DurationVar(&crawlCfg.MaxDuration, "max-duration", 5*time.Minute, "overall crawl timeout")
	crawlCmd.Flags().Int64Var(&crawlCfg.MaxBodySize, "max-body-size", crawler.DefaultMaxBodySize, "max response body bytes to read per page")
	crawlCmd.Flags().StringVar(&crawlCfg.Output, "output", "text", "text|json")
	crawlCmd.Flags().IntVar(&crawlCfg.FailBelow, "fail-below", 0, "exit 1 if score below this")
	crawlCmd.Flags().StringVar(&crawlCfg.Baseline, "baseline", "", "path to baseline JSON report")
	crawlCmd.Flags().IntVar(&crawlCfg.MaxScoreDrop, "max-score-drop", 5, "tolerated score drop vs --baseline")
	crawlCmd.Flags().StringSliceVar(&crawlCfg.StrictRules, "strict-rules", regression.DefaultStrictRules(),
		`SEO checks (e.g. SINGLE_H1) that fail the build on new or worse failures even when score drop is within --max-score-drop ("" disables)`)
	crawlCmd.MarkFlagRequired("url")
	rootCmd.AddCommand(crawlCmd)
}
