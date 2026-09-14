package cmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/Erose112/seo-audit/internal/config"
	"github.com/Erose112/seo-audit/internal/crawler"
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

		ctx, cancel := crawler.NewCrawlContext(crawlCfg.MaxDuration)
		defer cancel()

		rep, err := runCrawl(ctx, crawlCfg)
		if err != nil {
			return err
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
		return nil
	},
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
	pageScores := make([]int, 0, len(result.Pages))

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
		pageScores = append(pageScores, score)
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

func writeCrawlDiagnostics(w io.Writer, result crawler.CrawlResult) {
	fmt.Fprintf(w, "Pages crawled: %d\n", len(result.Pages))
	fmt.Fprintf(w, "Page errors:   %d\n", len(result.Errors))
	for _, p := range result.Pages {
		fmt.Fprintf(w, "  [%d] %s — %q\n", p.Depth, p.URL, p.Data.Title)
	}
	for _, e := range result.Errors {
		if e.StatusCode > 0 {
			fmt.Fprintf(w, "  error: %s — %s (%d): %s\n", e.URL, e.Kind, e.StatusCode, e.Message)
		} else {
			fmt.Fprintf(w, "  error: %s — %s: %s\n", e.URL, e.Kind, e.Message)
		}
	}
}

func init() {
	crawlCmd.Flags().StringVar(&crawlCfg.URL, "url", "", "target URL (required)")
	crawlCmd.Flags().IntVar(&crawlCfg.MaxPages, "max-pages", 100, "max pages to crawl")
	crawlCmd.Flags().IntVar(&crawlCfg.MaxDepth, "max-depth", 5, "max crawl depth")
	crawlCmd.Flags().DurationVar(&crawlCfg.Delay, "delay", 200*time.Millisecond, "delay between requests")
	crawlCmd.Flags().DurationVar(&crawlCfg.MaxDuration, "max-duration", 5*time.Minute, "max wall-clock time for the crawl")
	crawlCmd.Flags().Int64Var(&crawlCfg.MaxBodySize, "max-body-size", crawler.DefaultMaxBodySize, "max response body bytes to read per page")
	crawlCmd.Flags().StringVar(&crawlCfg.Output, "output", "text", "text|json")
	crawlCmd.MarkFlagRequired("url")
	rootCmd.AddCommand(crawlCmd)
}
