package cmd

import (
	"fmt"
	"time"

	"github.com/Erose112/seo-audit/internal/config"
	"github.com/Erose112/seo-audit/internal/crawler"
	"github.com/spf13/cobra"
)

var crawlCfg config.CrawlConfig

var crawlCmd = &cobra.Command{
	Use:          "crawl",
	Short:        "Crawl a site and produce an SEO audit report",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := crawler.NewCrawlContext(crawlCfg.MaxDuration)
		defer cancel()

		cfg := crawler.DefaultConfig()
		if crawlCfg.MaxBodySize > 0 {
			cfg.MaxBodySize = crawlCfg.MaxBodySize
		}

		fetcher, err := crawler.NewFetcher(crawler.NewClient(), cfg)
		if err != nil {
			return err
		}

		result, err := crawler.Crawl(ctx, fetcher, crawlCfg.URL, crawler.CrawlOptions{
			MaxPages: crawlCfg.MaxPages,
			MaxDepth: crawlCfg.MaxDepth,
			Delay:    crawlCfg.Delay,
		})
		if err != nil {
			return err
		}

		printCrawlSummary(cmd, result)
		return nil
	},
}

func printCrawlSummary(cmd *cobra.Command, result crawler.CrawlResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Pages crawled: %d\n", len(result.Pages))
	fmt.Fprintf(out, "Page errors:   %d\n", len(result.Errors))
	for _, p := range result.Pages {
		fmt.Fprintf(out, "  [%d] %s — %q\n", p.Depth, p.URL, p.Data.Title)
	}
	for _, e := range result.Errors {
		if e.StatusCode > 0 {
			fmt.Fprintf(out, "  error: %s — %s (%d): %s\n", e.URL, e.Kind, e.StatusCode, e.Message)
		} else {
			fmt.Fprintf(out, "  error: %s — %s: %s\n", e.URL, e.Kind, e.Message)
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
