package cmd

import (
	"fmt"
	"time"

	"github.com/Erose112/seo-audit/internal/config"
	"github.com/Erose112/seo-audit/internal/crawler"
	"github.com/Erose112/seo-audit/internal/parser"
	"github.com/spf13/cobra"
)

var crawlCfg config.CrawlConfig

var crawlCmd = &cobra.Command{
	Use:          "crawl",
	Short:        "Crawl a site and produce an SEO audit report",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Stage 2: single-page fetch and parse. The frontier, robots.txt and
		// worker pool replace this in Stage 6.
		cfg := crawler.DefaultConfig()
		if crawlCfg.MaxBodySize > 0 {
			cfg.MaxBodySize = crawlCfg.MaxBodySize
		}

		fetcher := crawler.NewFetcher(crawler.NewClient(cfg), cfg)
		page, err := fetcher.FetchWithRetry(cmd.Context(), crawlCfg.URL, cfg.Retry)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", crawlCfg.URL, err)
		}
		if !crawler.IsHTML(page.ContentType) {
			return fmt.Errorf("fetch %s: content type %q is not HTML", crawlCfg.URL, page.ContentType)
		}
		if page.Truncated {
			return fmt.Errorf("fetch %s: body exceeds the %d byte limit", crawlCfg.URL, cfg.MaxBodySize)
		}

		data, err := parser.Parse(page.FinalURL, page.Body)
		if err != nil {
			return err
		}
		printPageData(cmd, page, data)
		return nil
	},
}

func printPageData(cmd *cobra.Command, page *crawler.PageResponse, data parser.PageData) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "URL:              %s\n", data.URL)
	fmt.Fprintf(out, "Status:           %d (%s, %s)\n", page.StatusCode, page.ContentType, page.Duration.Round(time.Millisecond))
	fmt.Fprintf(out, "Title:            %q\n", data.Title)
	fmt.Fprintf(out, "Meta description: %q\n", data.MetaDescription)
	fmt.Fprintf(out, "Canonical:        %q\n", data.Canonical)
	fmt.Fprintf(out, "Viewport:         %q\n", data.Viewport)
	fmt.Fprintf(out, "H1 count:         %d %v\n", data.H1Count, data.H1Text)
	fmt.Fprintf(out, "Word count:       %d\n", data.WordCount)

	missingAlt := 0
	for _, img := range data.Images {
		if img.Alt == "" {
			missingAlt++
		}
	}
	fmt.Fprintf(out, "Images:           %d (%d missing alt)\n", len(data.Images), missingAlt)

	internal := 0
	for _, link := range data.Links {
		if link.Internal {
			internal++
		}
	}
	fmt.Fprintf(out, "Links:            %d (%d internal, %d external)\n", len(data.Links), internal, len(data.Links)-internal)
}

func init() {
	crawlCmd.Flags().StringVar(&crawlCfg.URL, "url", "", "target URL (required)")
	crawlCmd.Flags().IntVar(&crawlCfg.MaxPages, "max-pages", 100, "max pages to crawl")
	crawlCmd.Flags().IntVar(&crawlCfg.MaxDepth, "max-depth", 5, "max crawl depth")
	crawlCmd.Flags().DurationVar(&crawlCfg.Delay, "delay", 200*time.Millisecond, "delay between requests")
	crawlCmd.Flags().Int64Var(&crawlCfg.MaxBodySize, "max-body-size", crawler.DefaultMaxBodySize, "max response body bytes to read per page")
	crawlCmd.Flags().StringVar(&crawlCfg.Output, "output", "text", "text|json")
	crawlCmd.MarkFlagRequired("url")
	rootCmd.AddCommand(crawlCmd)
}
