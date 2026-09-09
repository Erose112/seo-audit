package parser

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// PageData is everything the checks need from one HTML document. Nothing
// downstream of the parser reads raw HTML.
type PageData struct {
	URL             string
	Title           string
	MetaDescription string
	H1Count         int
	H1Text          []string
	Images          []Image
	Canonical       string
	Viewport        string
	Links           []Link
	WordCount       int
}

type Image struct {
	Src, Alt string
}

type Link struct {
	Href     string
	Internal bool
	Text     string
}

// Parse extracts PageData from an HTML document. baseURL is the URL the
// document was actually served from (after redirects), since relative links and
// image sources resolve against it.
//
// Internal-vs-external classification lives here because both the crawl
// frontier and the broken-link check need it, and both read PageData.
func Parse(baseURL string, body []byte) (PageData, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return PageData{}, fmt.Errorf("parse base URL %q: %w", baseURL, err)
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return PageData{}, fmt.Errorf("parse HTML for %s: %w", baseURL, err)
	}

	data := PageData{URL: baseURL}
	data.Title = strings.TrimSpace(doc.Find("title").First().Text())
	data.MetaDescription = metaContent(doc, "description")
	data.Viewport = metaContent(doc, "viewport")
	data.Canonical = canonical(doc, base)

	doc.Find("h1").Each(func(_ int, s *goquery.Selection) {
		data.H1Count++
		data.H1Text = append(data.H1Text, strings.Join(strings.Fields(s.Text()), " "))
	})

	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		alt, _ := s.Attr("alt")
		src = strings.TrimSpace(src)
		// A data: or otherwise unresolvable src still needs alt text, so keep
		// the raw value rather than dropping the image from the results.
		if abs, err := resolve(base, src); err == nil {
			src = abs.String()
		}
		data.Images = append(data.Images, Image{Src: src, Alt: strings.TrimSpace(alt)})
	})

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		abs, err := resolve(base, href)
		if err != nil {
			return
		}
		// mailto:, tel: and javascript: aren't crawlable and aren't link
		// targets the audit can check.
		if abs.Scheme != "http" && abs.Scheme != "https" {
			return
		}
		data.Links = append(data.Links, Link{
			Href:     abs.String(),
			Internal: sameHost(base, abs),
			Text:     strings.Join(strings.Fields(s.Text()), " "),
		})
	})

	data.WordCount = wordCount(doc)
	return data, nil
}

// metaContent looks up a <meta name="..."> by case-insensitive name, since
// real-world pages ship "Description" and "DESCRIPTION" as often as not.
func metaContent(doc *goquery.Document, name string) string {
	var content string
	doc.Find("meta[name]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		attr, _ := s.Attr("name")
		if !strings.EqualFold(strings.TrimSpace(attr), name) {
			return true
		}
		value, _ := s.Attr("content")
		content = strings.TrimSpace(value)
		return false
	})
	return content
}

func canonical(doc *goquery.Document, base *url.URL) string {
	var href string
	doc.Find("link[rel][href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		rel, _ := s.Attr("rel")
		if !strings.EqualFold(strings.TrimSpace(rel), "canonical") {
			return true
		}
		value, _ := s.Attr("href")
		if abs, err := resolve(base, value); err == nil {
			href = abs.String()
		} else {
			href = strings.TrimSpace(value)
		}
		return false
	})
	return href
}

func resolve(base *url.URL, ref string) (*url.URL, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty reference")
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(parsed), nil
}

// sameHost compares hosts exactly apart from case. www.example.com and
// example.com are deliberately not merged; see the URL normalization rules.
func sameHost(a, b *url.URL) bool {
	return strings.EqualFold(a.Host, b.Host)
}

func wordCount(doc *goquery.Document) int {
	body := doc.Find("body")
	if body.Length() == 0 {
		return 0
	}
	body.Find("script, style, noscript, template").Remove()
	return len(strings.Fields(body.Text()))
}
