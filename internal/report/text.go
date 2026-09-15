package report

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/fatih/color"
)

func (r Report) WriteText(w io.Writer) error {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)
	bold := color.New(color.Bold)

	if _, err := bold.Fprintln(w, "SEO AUDIT"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("━", 26)); err != nil {
		return err
	}

	for _, check := range r.Checks {
		symbol, c := checkSymbol(check.Severity, green, red, yellow)
		if _, err := c.Fprintf(w, "%s %s\n", symbol, check.Name); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "\nScore: %d/100\n", r.Score); err != nil {
		return err
	}

	if err := writeFailedChecks(w, r.Pages); err != nil {
		return err
	}
	if err := writeSiteIssues(w, r.SiteIssues); err != nil {
		return err
	}
	return nil
}

func checkSymbol(severity checks.Severity, green, red, yellow *color.Color) (string, *color.Color) {
	switch severity {
	case checks.Error:
		return "✗", red
	case checks.Warning:
		return "⚠", yellow
	default:
		return "✓", green
	}
}

func writeFailedChecks(w io.Writer, pages []PageReport) error {
	var hasFailures bool
	for _, page := range pages {
		for _, result := range page.Results {
			if result.Severity == checks.Warning || result.Severity == checks.Error {
				hasFailures = true
				break
			}
		}
		if hasFailures {
			break
		}
	}
	if !hasFailures {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nFAILED CHECKS"); err != nil {
		return err
	}

	for _, page := range pages {
		var messages []string
		for _, result := range page.Results {
			if result.Severity == checks.Warning || result.Severity == checks.Error {
				messages = append(messages, result.Message)
			}
		}
		if len(messages) == 0 {
			continue
		}
		path := PagePath(page.URL)
		if _, err := fmt.Fprintf(w, "- %s\n", path); err != nil {
			return err
		}
		for _, msg := range messages {
			if _, err := fmt.Fprintf(w, "  %s\n", msg); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeSiteIssues(w io.Writer, site checks.SiteResult) error {
	if len(site.DuplicateTitles) == 0 && len(site.BrokenLinks) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nSITE ISSUES"); err != nil {
		return err
	}

	for title, urls := range site.DuplicateTitles {
		if _, err := fmt.Fprintf(w, "- Duplicate title %q on:\n", title); err != nil {
			return err
		}
		for _, u := range urls {
			if _, err := fmt.Fprintf(w, "  %s\n", PagePath(u)); err != nil {
				return err
			}
		}
	}

	for _, bl := range site.BrokenLinks {
		line := fmt.Sprintf("- Broken link from %s to %s (%s)", PagePath(bl.SourceURL), PagePath(bl.TargetURL), bl.Kind)
		if bl.StatusCode != 0 {
			line += fmt.Sprintf(" %d", bl.StatusCode)
		}
		if bl.Message != "" {
			line += ": " + bl.Message
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// PagePath renders a URL as a path for terminal output (/ when empty).
func PagePath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Path == "" {
		return "/"
	}
	return u.Path
}
