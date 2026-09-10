package checks

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Erose112/seo-audit/internal/parser"
)

// Thresholds are named because Stage 12 tunes them against real pages.
const (
	titleMinLen = 30
	titleMaxLen = 60
)

type TitleExistsCheck struct{}

func (TitleExistsCheck) ID() string   { return "TITLE_EXISTS" }
func (TitleExistsCheck) Name() string { return "Title tag exists" }

func (c TitleExistsCheck) Run(p parser.PageData) CheckResult {
	if strings.TrimSpace(p.Title) == "" {
		return CheckResult{c.ID(), Error, "Missing <title> tag", 10}
	}
	return CheckResult{c.ID(), Pass, "", 0}
}

type TitleLengthCheck struct{}

func (TitleLengthCheck) ID() string   { return "TITLE_LENGTH" }
func (TitleLengthCheck) Name() string { return "Title length within range" }

func (c TitleLengthCheck) Run(p parser.PageData) CheckResult {
	title := strings.TrimSpace(p.Title)
	if title == "" {
		// TITLE_EXISTS already penalizes a missing title; passing here keeps
		// one problem from deducting twice.
		return CheckResult{c.ID(), Pass, "", 0}
	}
	// Characters, not bytes: len() undercounts accented, CJK and emoji titles.
	n := utf8.RuneCountInString(title)
	if n < titleMinLen || n > titleMaxLen {
		return CheckResult{
			c.ID(), Warning,
			fmt.Sprintf("Title is %d characters (recommended: %d-%d)", n, titleMinLen, titleMaxLen),
			5,
		}
	}
	return CheckResult{c.ID(), Pass, "", 0}
}
