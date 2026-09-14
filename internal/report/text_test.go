package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Erose112/seo-audit/internal/checks"
	"github.com/fatih/color"
)

func TestWriteTextSections(t *testing.T) {
	old := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = old })

	rep := goldenReport()
	var buf bytes.Buffer
	if err := rep.WriteText(&buf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"SEO AUDIT",
		"Score: 75/100",
		"FAILED CHECKS",
		"/home",
		"Multiple H1 tags",
		"SITE ISSUES",
		"Duplicate title",
		"Broken link",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestWriteTextNoANSIWhenNotTTY(t *testing.T) {
	rep := Report{
		Checks: []CheckSummary{
			{CheckID: "TITLE_EXISTS", Name: "Title tag exists", Severity: checks.Pass},
			{CheckID: "SINGLE_H1", Name: "Exactly one H1 tag", Severity: checks.Error},
		},
		Score: 90,
		Pages: []PageReport{
			{
				URL: "https://example.com/bad",
				Results: []checks.CheckResult{
					{CheckID: "SINGLE_H1", Severity: checks.Error, Message: "Multiple H1 tags"},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := rep.WriteText(&buf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("\x1b[")) {
		t.Errorf("expected no ANSI escapes when writing to a buffer, got: %q", buf.Bytes())
	}
}
