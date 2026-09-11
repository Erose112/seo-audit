package checks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Erose112/seo-audit/internal/parser"
)

const testBase = "https://example.com/page"

// loadFixture parses one of the shared Stage 2 HTML fixtures into PageData, so
// checks are exercised against real parser output rather than hand-built structs.
func loadFixture(t *testing.T, name string) parser.PageData {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	data, err := parser.Parse(testBase, body)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return data
}

func TestAllChecksRegistry(t *testing.T) {
	all := AllChecks()
	if len(all) != 7 {
		t.Fatalf("expected 7 checks, got %d", len(all))
	}
	seen := make(map[string]bool, len(all))
	for _, c := range all {
		if c == nil {
			t.Fatal("nil check in registry")
		}
		id := c.ID()
		if id == "" {
			t.Fatalf("check %T has empty ID", c)
		}
		if seen[id] {
			t.Fatalf("duplicate check ID %q", id)
		}
		seen[id] = true
		if c.Name() == "" {
			t.Errorf("check %q has empty Name", id)
		}
	}
}

// Every result must carry its own ID and keep severity and deduction
// consistent, since Stage 5 sums deductions without inspecting severity.
func TestAllChecksResultInvariants(t *testing.T) {
	fixtures := []string{
		"valid_page.html", "missing_title.html", "multiple_h1.html",
		"missing_alt.html", "malformed.html", "missing_canonical.html",
		"no_viewport.html",
	}
	for _, f := range fixtures {
		page := loadFixture(t, f)
		for _, c := range AllChecks() {
			result := c.Run(page)
			if result.CheckID != c.ID() {
				t.Errorf("%s/%s: CheckID = %q, want %q", f, c.ID(), result.CheckID, c.ID())
			}
			if result.Deduction < 0 {
				t.Errorf("%s/%s: negative deduction %d", f, c.ID(), result.Deduction)
			}
			if result.Severity == Pass && result.Deduction != 0 {
				t.Errorf("%s/%s: pass with deduction %d", f, c.ID(), result.Deduction)
			}
			if result.Severity != Pass && result.Deduction == 0 {
				t.Errorf("%s/%s: %s with no deduction", f, c.ID(), result.Severity)
			}
			if result.Severity != Pass && result.Message == "" {
				t.Errorf("%s/%s: %s with no message", f, c.ID(), result.Severity)
			}
		}
	}
}

// The worst-case total bounds what Stage 5 can subtract from 100.
func TestWorstCaseDeductionCeiling(t *testing.T) {
	worst := parser.PageData{
		Title:           "",
		MetaDescription: "",
		H1Count:         0,
		Images:          []parser.Image{{Src: "a.png"}},
		Canonical:       "",
		Viewport:        "",
	}
	total := 0
	for _, c := range AllChecks() {
		total += c.Run(worst).Deduction
	}
	if total != 50 {
		t.Errorf("worst-case deduction = %d, want 50", total)
	}
}
