package config

import "time"

type CrawlConfig struct {
	URL         string
	MaxPages    int
	MaxDepth    int
	Delay       time.Duration
	MaxDuration time.Duration
	MaxBodySize int64
	Output      string // "text" | "json"
	FailBelow   int
	Baseline    string // path to previous report, empty = none
}

type CompareConfig struct {
	Baseline     string
	Current      string
	FailBelow    int
	MaxScoreDrop int
	StrictRules  []string
	TextMatchPct int
}
