package parser

import (
	"os"
	"path/filepath"
	"testing"
)

const testBase = "https://example.com/page"

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func TestParse(t *testing.T) {
	cases := []struct {
		name           string
		file           string
		wantTitle      string
		wantH1Count    int
		wantImages     int
		wantMissingAlt int
		wantLinks      int
		wantInternal   int
		wantCanonical  string
		wantViewport   bool
		wantMetaDesc   bool
	}{
		{
			name:          "valid page",
			file:          "valid_page.html",
			wantTitle:     "Example Title",
			wantH1Count:   1,
			wantImages:    2,
			wantLinks:     3, // mailto: is not a crawlable link
			wantInternal:  2,
			wantCanonical: "https://example.com/valid",
			wantViewport:  true,
			wantMetaDesc:  true,
		},
		{
			name:          "missing title",
			file:          "missing_title.html",
			wantTitle:     "",
			wantH1Count:   1,
			wantImages:    1,
			wantLinks:     1,
			wantInternal:  1,
			wantCanonical: "https://example.com/missing-title",
			wantViewport:  true,
			wantMetaDesc:  true,
		},
		{
			name:          "multiple h1",
			file:          "multiple_h1.html",
			wantTitle:     "Example Title",
			wantH1Count:   2,
			wantImages:    1,
			wantLinks:     1,
			wantInternal:  1,
			wantCanonical: "https://example.com/multiple-h1",
			wantViewport:  true,
			wantMetaDesc:  true,
		},
		{
			name:           "missing alt text",
			file:           "missing_alt.html",
			wantTitle:      "Missing Alt Text",
			wantH1Count:    1,
			wantImages:     3,
			wantMissingAlt: 2,
			wantLinks:      1,
			wantInternal:   1,
			wantCanonical:  "https://example.com/missing-alt",
			wantViewport:   true,
			wantMetaDesc:   true,
		},
		{
			name:          "malformed html",
			file:          "malformed.html",
			wantTitle:     "Malformed Page",
			wantH1Count:   1,
			wantImages:    1,
			wantLinks:     1,
			wantInternal:  1,
			wantCanonical: "https://example.com/malformed",
			wantViewport:  true,
			wantMetaDesc:  true,
		},
		{
			name:          "missing canonical",
			file:          "missing_canonical.html",
			wantTitle:     "Missing Canonical",
			wantH1Count:   1,
			wantImages:    1,
			wantLinks:     1,
			wantInternal:  1,
			wantCanonical: "",
			wantViewport:  true,
			wantMetaDesc:  true,
		},
		{
			name:          "no viewport",
			file:          "no_viewport.html",
			wantTitle:     "No Viewport",
			wantH1Count:   1,
			wantImages:    1,
			wantLinks:     1,
			wantInternal:  1,
			wantCanonical: "https://example.com/no-viewport",
			wantViewport:  false,
			wantMetaDesc:  true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := Parse(testBase, loadFixture(t, c.file))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if data.Title != c.wantTitle {
				t.Errorf("Title = %q, want %q", data.Title, c.wantTitle)
			}
			if data.H1Count != c.wantH1Count {
				t.Errorf("H1Count = %d, want %d", data.H1Count, c.wantH1Count)
			}
			if len(data.H1Text) != data.H1Count {
				t.Errorf("len(H1Text) = %d, want %d", len(data.H1Text), data.H1Count)
			}
			if len(data.Images) != c.wantImages {
				t.Errorf("len(Images) = %d, want %d", len(data.Images), c.wantImages)
			}

			missingAlt := 0
			for _, img := range data.Images {
				if img.Alt == "" {
					missingAlt++
				}
			}
			if missingAlt != c.wantMissingAlt {
				t.Errorf("images missing alt = %d, want %d", missingAlt, c.wantMissingAlt)
			}

			if len(data.Links) != c.wantLinks {
				t.Errorf("len(Links) = %d, want %d (%v)", len(data.Links), c.wantLinks, data.Links)
			}
			internal := 0
			for _, l := range data.Links {
				if l.Internal {
					internal++
				}
			}
			if internal != c.wantInternal {
				t.Errorf("internal links = %d, want %d", internal, c.wantInternal)
			}

			if data.Canonical != c.wantCanonical {
				t.Errorf("Canonical = %q, want %q", data.Canonical, c.wantCanonical)
			}
			if got := data.Viewport != ""; got != c.wantViewport {
				t.Errorf("viewport present = %v, want %v", got, c.wantViewport)
			}
			if got := data.MetaDescription != ""; got != c.wantMetaDesc {
				t.Errorf("meta description present = %v, want %v", got, c.wantMetaDesc)
			}
			if data.URL != testBase {
				t.Errorf("URL = %q, want %q", data.URL, testBase)
			}
		})
	}
}

func TestParseResolvesLinksAgainstBase(t *testing.T) {
	data, err := Parse("https://example.com/valid", loadFixture(t, "valid_page.html"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want := map[string]bool{
		"https://example.com/about":         true,
		"https://example.com/contact/":      true,
		"https://external.example.org/docs": false,
	}
	if len(data.Links) != len(want) {
		t.Fatalf("len(Links) = %d, want %d (%v)", len(data.Links), len(want), data.Links)
	}
	for _, l := range data.Links {
		internal, ok := want[l.Href]
		if !ok {
			t.Errorf("unexpected link %q", l.Href)
			continue
		}
		if l.Internal != internal {
			t.Errorf("link %q internal = %v, want %v", l.Href, l.Internal, internal)
		}
	}

	if len(data.Images) != 2 {
		t.Fatalf("len(Images) = %d, want 2", len(data.Images))
	}
	if data.Images[0].Src != "https://example.com/img/one.png" {
		t.Errorf("Images[0].Src = %q, want absolute resolution against the base", data.Images[0].Src)
	}
	if data.Images[1].Src != "https://cdn.example.net/two.png" {
		t.Errorf("Images[1].Src = %q, want the absolute src unchanged", data.Images[1].Src)
	}
}

func TestParseWordCountExcludesScriptAndStyle(t *testing.T) {
	html := []byte(`<html><head><title>T</title><style>body { color: red; }</style></head>` +
		`<body><p>one two three</p><script>four five six</script></body></html>`)

	data, err := Parse(testBase, html)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if data.WordCount != 3 {
		t.Errorf("WordCount = %d, want 3", data.WordCount)
	}
}

func TestParseMetaNameIsCaseInsensitive(t *testing.T) {
	html := []byte(`<html><head><meta name="Description" content="Mixed case name attribute.">` +
		`<link rel="Canonical" href="/canonical"></head><body></body></html>`)

	data, err := Parse(testBase, html)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if data.MetaDescription != "Mixed case name attribute." {
		t.Errorf("MetaDescription = %q, want the mixed-case meta to be found", data.MetaDescription)
	}
	if data.Canonical != "https://example.com/canonical" {
		t.Errorf("Canonical = %q, want the mixed-case rel to be found and resolved", data.Canonical)
	}
}

func TestParseInvalidBaseURL(t *testing.T) {
	if _, err := Parse("://not-a-url", []byte("<html></html>")); err == nil {
		t.Fatal("Parse: want an error for an unparseable base URL, got nil")
	}
}
