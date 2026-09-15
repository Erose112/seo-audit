package cmd

import (
	"errors"
	"testing"

	"github.com/Erose112/seo-audit/internal/config"
	"github.com/Erose112/seo-audit/internal/report"
)

func TestExitCodeFor(t *testing.T) {
	t.Parallel()

	runErr := errors.New("root URL unreachable")

	tests := []struct {
		name      string
		rep       report.Report
		cfg       config.CrawlConfig
		runErr    error
		regressed bool
		want      int
	}{
		{
			name:   "crawl error",
			runErr: runErr,
			want:   2,
		},
		{
			name:   "score below gate",
			rep:    report.Report{Score: 79},
			cfg:    config.CrawlConfig{FailBelow: 80},
			want:   1,
		},
		{
			name:   "score at gate",
			rep:    report.Report{Score: 80},
			cfg:    config.CrawlConfig{FailBelow: 80},
			want:   0,
		},
		{
			name:   "score above gate",
			rep:    report.Report{Score: 95},
			cfg:    config.CrawlConfig{FailBelow: 80},
			want:   0,
		},
		{
			name:   "fail below zero disables gate",
			rep:    report.Report{Score: 0},
			cfg:    config.CrawlConfig{FailBelow: 0},
			want:   0,
		},
		{
			name:      "regression without score gate",
			rep:       report.Report{Score: 95},
			cfg:       config.CrawlConfig{FailBelow: 80},
			regressed: true,
			want:      1,
		},
		{
			name:      "regression and score gate both fail",
			rep:       report.Report{Score: 70},
			cfg:       config.CrawlConfig{FailBelow: 80},
			regressed: true,
			want:      1,
		},
		{
			name:      "regression false score pass",
			rep:       report.Report{Score: 95},
			cfg:       config.CrawlConfig{FailBelow: 80},
			regressed: false,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := exitCodeFor(tt.rep, tt.cfg, tt.runErr, tt.regressed); got != tt.want {
				t.Errorf("exitCodeFor() = %d, want %d", got, tt.want)
			}
		})
	}
}
