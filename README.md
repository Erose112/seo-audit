# seo-audit

## URL normalization (v1)

The crawler deduplicates pages using a canonical URL key produced by
`internal/crawler.Normalize`. v1 rules:

- **Fragments** are stripped (`/page#section` → `/page`).
- **Scheme and host** are lowercased; default ports are removed (`:80` for
  `http`, `:443` for `https`).
- **Percent-encoding** follows RFC 3986 §6.2.2.2: unreserved characters are
  decoded, remaining percent-encoded hex is uppercased.
- **Path casing**, **trailing slashes** (`/about` vs `/about/`), **query
  strings** (`?id=5` vs `?id=5&utm_source=x`), and **`www.` vs bare domain**
  are left distinct — no merging.

Known limitations for v1:

- Tracking query params (`utm_*`, `fbclid`, session IDs) can inflate crawl
  budget with near-duplicate URLs.
- Sites that serve the same content at both `/about` and `/about/` without
  redirects or canonical tags may be crawled twice.
- `www.example.com` and `example.com` are treated as different hosts unless
  the crawl resolves a single canonical host via redirects (Stage 6).