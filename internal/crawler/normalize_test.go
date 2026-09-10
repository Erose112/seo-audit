package crawler

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestResolveURL(t *testing.T) {
	const base = "https://example.com/blog/post/"

	cases := []struct {
		name    string
		base    string
		ref     string
		want    string
		wantErr bool
	}{
		{
			name: "relative path",
			base: base,
			ref:  "about",
			want: "https://example.com/blog/post/about",
		},
		{
			name: "relative with parent segment",
			base: base,
			ref:  "../archive",
			want: "https://example.com/blog/archive",
		},
		{
			name: "protocol-relative",
			base: base,
			ref:  "//cdn.example.com/assets/logo.png",
			want: "https://cdn.example.com/assets/logo.png",
		},
		{
			name: "absolute href passthrough",
			base: base,
			ref:  "https://other.example/page",
			want: "https://other.example/page",
		},
		{
			name: "root-relative path",
			base: base,
			ref:  "/contact",
			want: "https://example.com/contact",
		},
		{
			name:    "bad base",
			base:    "://bad",
			ref:     "about",
			wantErr: true,
		},
		{
			name:    "bad ref",
			base:    base,
			ref:     "://bad",
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ResolveURL(c.base, c.ref)
			if c.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveURL() error = %v", err)
			}
			if got != c.want {
				t.Errorf("ResolveURL() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "strip fragment",
			in:   "https://example.com/page#section",
			want: "https://example.com/page",
		},
		{
			name: "trailing slash kept without slash",
			in:   "https://example.com/about",
			want: "https://example.com/about",
		},
		{
			name: "trailing slash kept with slash",
			in:   "https://example.com/about/",
			want: "https://example.com/about/",
		},
		{
			name: "trailing slash variants stay distinct",
			in:   "https://example.com/about",
			want: "https://example.com/about",
		},
		{
			name: "query strings left intact",
			in:   "https://example.com/search?q=go",
			want: "https://example.com/search?q=go",
		},
		{
			name: "different query strings stay distinct",
			in:   "https://example.com/page?id=5&utm_source=x",
			want: "https://example.com/page?id=5&utm_source=x",
		},
		{
			name: "lowercase scheme and host",
			in:   "HTTP://EXAMPLE.COM/Foo",
			want: "http://example.com/Foo",
		},
		{
			name: "strip default http port",
			in:   "http://example.com:80/path",
			want: "http://example.com/path",
		},
		{
			name: "strip default https port",
			in:   "https://Example.com:443/path",
			want: "https://example.com/path",
		},
		{
			name: "keep non-default port",
			in:   "https://example.com:8443/path",
			want: "https://example.com:8443/path",
		},
		{
			name: "percent-encoded path decoded for unreserved",
			in:   "https://example.com/caf%C3%A9",
			want: "https://example.com/caf%C3%A9",
		},
		{
			name: "percent hex uppercased",
			in:   "https://example.com/a%2fb",
			want: "https://example.com/a%2Fb",
		},
		{
			name: "www and bare stay distinct",
			in:   "https://www.example.com/page",
			want: "https://www.example.com/page",
		},
		{
			name: "root path unchanged",
			in:   "https://example.com/",
			want: "https://example.com/",
		},
		{
			name:    "relative URL rejected",
			in:      "/about",
			wantErr: true,
		},
		{
			name:    "malformed URL rejected",
			in:      "://bad",
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Normalize(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize() error = %v", err)
			}
			if got != c.want {
				t.Errorf("Normalize() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalizePercentEncodingEquivalence(t *testing.T) {
	encoded, err := Normalize("https://example.com/caf%C3%A9")
	if err != nil {
		t.Fatalf("Normalize(encoded) error = %v", err)
	}
	unicode, err := Normalize("https://example.com/café")
	if err != nil {
		t.Fatalf("Normalize(unicode) error = %v", err)
	}
	if encoded != unicode {
		t.Errorf("encoded and unicode paths differ: %q vs %q", encoded, unicode)
	}
}

func TestNormalizeTrailingSlashDistinct(t *testing.T) {
	withSlash, err := Normalize("https://example.com/about/")
	if err != nil {
		t.Fatalf("Normalize(with slash) error = %v", err)
	}
	withoutSlash, err := Normalize("https://example.com/about")
	if err != nil {
		t.Fatalf("Normalize(without slash) error = %v", err)
	}
	if withSlash == withoutSlash {
		t.Errorf("trailing slash variants merged: both %q", withSlash)
	}
}

func TestNormalizeQueryDistinct(t *testing.T) {
	a, err := Normalize("https://example.com/page?id=5")
	if err != nil {
		t.Fatalf("Normalize(a) error = %v", err)
	}
	b, err := Normalize("https://example.com/page?id=5&utm_source=x")
	if err != nil {
		t.Fatalf("Normalize(b) error = %v", err)
	}
	if a == b {
		t.Errorf("query variants merged: both %q", a)
	}
}

func TestNormalizeWWWDistinct(t *testing.T) {
	www, err := Normalize("https://www.example.com/page")
	if err != nil {
		t.Fatalf("Normalize(www) error = %v", err)
	}
	bare, err := Normalize("https://example.com/page")
	if err != nil {
		t.Fatalf("Normalize(bare) error = %v", err)
	}
	if www == bare {
		t.Errorf("www and bare merged: both %q", www)
	}
}

func TestSameDomain(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{
			name: "same host after casing normalize",
			a:    "https://Example.com/page",
			b:    "https://example.com/other",
			want: true,
		},
		{
			name: "same host after default port strip",
			a:    "https://example.com:443/a",
			b:    "https://example.com/b",
			want: true,
		},
		{
			name: "www vs bare are different",
			a:    "https://www.example.com/a",
			b:    "https://example.com/b",
			want: false,
		},
		{
			name: "different hosts",
			a:    "https://example.com/a",
			b:    "https://other.com/b",
			want: false,
		},
		{
			name: "bad first URL",
			a:    "not-a-url",
			b:    "https://example.com/",
			want: false,
		},
		{
			name: "bad second URL",
			a:    "https://example.com/",
			b:    "not-a-url",
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SameDomain(c.a, c.b); got != c.want {
				t.Errorf("SameDomain(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestResolveAndNormalizeFromHTML(t *testing.T) {
	const pageURL = "https://example.com/blog/post/"
	const htmlDoc = `<!DOCTYPE html>
<html>
<head><title>Post</title></head>
<body>
  <a href="about">About</a>
  <a href="../archive">Archive</a>
  <a href="//cdn.example.com/logo.png">Logo</a>
  <a href="/contact">Contact</a>
  <a href="https://example.com/page#top">Fragment</a>
</body>
</html>`

	want := map[string]string{
		"about":                          "https://example.com/blog/post/about",
		"../archive":                     "https://example.com/blog/archive",
		"//cdn.example.com/logo.png":     "https://cdn.example.com/logo.png",
		"/contact":                       "https://example.com/contact",
		"https://example.com/page#top": "https://example.com/page",
	}

	var hrefs []string
	z := html.NewTokenizer(strings.NewReader(htmlDoc))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		tok := z.Token()
		if tok.Data != "a" {
			continue
		}
		for _, attr := range tok.Attr {
			if attr.Key == "href" {
				hrefs = append(hrefs, attr.Val)
			}
		}
	}

	if len(hrefs) != len(want) {
		t.Fatalf("found %d hrefs, want %d", len(hrefs), len(want))
	}

	for _, href := range hrefs {
		expectedNorm, ok := want[href]
		if !ok {
			t.Fatalf("unexpected href %q in fixture", href)
		}

		resolved, err := ResolveURL(pageURL, href)
		if err != nil {
			t.Fatalf("ResolveURL(%q) error = %v", href, err)
		}

		norm, err := Normalize(resolved)
		if err != nil {
			t.Fatalf("Normalize(%q) error = %v", resolved, err)
		}

		if norm != expectedNorm {
			t.Errorf("href %q -> normalized %q, want %q", href, norm, expectedNorm)
		}
	}
}
