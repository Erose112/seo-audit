// Site-wide checks run after a crawl completes. They need the full CrawlResult,
// not a single PageData, so they live outside the per-page Check interface.
package checks

import (
	"sort"
	"strings"

	"github.com/Erose112/seo-audit/internal/crawler"
)

type SiteResult struct {
	DuplicateTitles map[string][]string // title -> URLs sharing it (2+ only)
	BrokenLinks     []BrokenLink
}

type BrokenLink struct {
	SourceURL  string
	TargetURL  string
	Kind       crawler.FetchErrorKind
	StatusCode int
	Message    string
}

// FindDuplicateTitles groups crawled pages by trimmed title and returns only
// titles shared by two or more pages. Empty/whitespace-only titles are excluded.
func FindDuplicateTitles(pages []crawler.CrawledPage) map[string][]string {
	groups := make(map[string][]string)
	for _, page := range pages {
		title := strings.TrimSpace(page.Data.Title)
		if title == "" {
			continue
		}
		groups[title] = append(groups[title], page.URL)
	}

	out := make(map[string][]string)
	for title, urls := range groups {
		if len(urls) < 2 {
			continue
		}
		sort.Strings(urls)
		out[title] = urls
	}
	return out
}

// FindBrokenLinks flags internal links whose normalized target appears in
// crawlErrors. Link resolution mirrors the frontier: ResolveURL + SameDomain,
// ignoring parser.Link.Internal. Only targets with a matching CrawlError are
// reported; uncrawled links (budget, robots, undiscovered) are not broken.
func FindBrokenLinks(pages []crawler.CrawledPage, crawlErrors []crawler.CrawlError) []BrokenLink {
	errorsByURL := make(map[string]crawler.CrawlError, len(crawlErrors))
	for _, ce := range crawlErrors {
		key, err := crawler.Normalize(ce.URL)
		if err != nil {
			continue
		}
		if _, exists := errorsByURL[key]; !exists {
			errorsByURL[key] = ce
		}
	}

	seen := make(map[string]struct{})
	var out []BrokenLink

	for _, page := range pages {
		for _, link := range page.Data.Links {
			resolved, err := crawler.ResolveURL(page.URL, link.Href)
			if err != nil {
				continue
			}
			if !crawler.SameDomain(page.URL, resolved) {
				continue
			}
			target, err := crawler.Normalize(resolved)
			if err != nil {
				continue
			}
			ce, ok := errorsByURL[target]
			if !ok {
				continue
			}

			dedupKey := page.URL + "\x00" + target
			if _, exists := seen[dedupKey]; exists {
				continue
			}
			seen[dedupKey] = struct{}{}

			out = append(out, BrokenLink{
				SourceURL:  page.URL,
				TargetURL:  target,
				Kind:       ce.Kind,
				StatusCode: ce.StatusCode,
				Message:    ce.Message,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceURL != out[j].SourceURL {
			return out[i].SourceURL < out[j].SourceURL
		}
		return out[i].TargetURL < out[j].TargetURL
	})
	return out
}
