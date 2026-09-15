package cmd

import (
	"os"

	"github.com/Erose112/seo-audit/internal/buildinfo"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "seo-audit",
	Short: "Crawl a site and evaluate SEO/technical-quality checks",
}

func init() {
	rootCmd.Version = buildinfo.String()
	rootCmd.SetVersionTemplate("{{.Name}} version {{.Version}}\n")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(2) // system/CLI error, per exit-code contract
	}
}
