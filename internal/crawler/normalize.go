package crawler

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ResolveURL turns a link href into an absolute URL using the page it was found
// on as the base. base must be the actual page URL (not the site root or seed
// URL) so relative paths and ../ segments resolve correctly.
//
// The result is not normalized; callers that need a visited-set key should pass
// the output through Normalize.
func ResolveURL(base, ref string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base URL %q: %w", base, err)
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("parse reference URL %q: %w", ref, err)
	}
	return baseURL.ResolveReference(refURL).String(), nil
}

// Normalize returns the canonical form of rawURL for use as a visited-set key
// and in reports. v1 rules:
//   - lowercase scheme and host
//   - strip default ports (80 for http, 443 for https)
//   - strip fragments
//   - normalize percent-encoding per RFC 3986 §6.2.2.2 (decode unreserved
//     characters only; uppercase remaining percent-encoded hex)
//   - leave path casing, trailing slashes, query strings, and www vs bare host
//     untouched (no merging)
func Normalize(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse URL %q: %w", rawURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("URL %q is not absolute", rawURL)
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = normalizeHost(u.Scheme, u.Host)
	u.Fragment = ""

	normalizedPath := normalizePercentEncoding(u.EscapedPath())
	decodedPath, err := url.PathUnescape(normalizedPath)
	if err != nil {
		return "", fmt.Errorf("decode path for %q: %w", rawURL, err)
	}
	u.Path = decodedPath
	u.RawPath = ""
	if u.EscapedPath() != normalizedPath {
		u.RawPath = normalizedPath
	}

	if u.RawQuery != "" {
		u.RawQuery = normalizePercentEncoding(u.RawQuery)
	}

	return u.String(), nil
}

// SameDomain reports whether a and b belong to the same host after
// normalization. www.example.com and example.com are treated as different
// hosts. Returns false if either URL cannot be normalized.
func SameDomain(a, b string) bool {
	na, err := Normalize(a)
	if err != nil {
		return false
	}
	nb, err := Normalize(b)
	if err != nil {
		return false
	}
	ua, err := url.Parse(na)
	if err != nil {
		return false
	}
	ub, err := url.Parse(nb)
	if err != nil {
		return false
	}
	return ua.Host == ub.Host
}

func normalizeHost(scheme, host string) string {
	hostname := host
	port := ""
	if h, p, err := net.SplitHostPort(host); err == nil {
		hostname = h
		port = p
	}
	hostname = strings.ToLower(hostname)

	if port == "" ||
		(scheme == "http" && port == "80") ||
		(scheme == "https" && port == "443") {
		return hostname
	}
	return net.JoinHostPort(hostname, port)
}

// normalizePercentEncoding applies RFC 3986 §6.2.2.2: decode percent-encoded
// unreserved characters and uppercase hex digits in all remaining sequences.
func normalizePercentEncoding(s string) string {
	if s == "" {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+2 >= len(s) {
			b.WriteByte(s[i])
			continue
		}

		hex := s[i+1 : i+3]
		if !isHexPair(hex) {
			b.WriteByte(s[i])
			continue
		}

		upper := strings.ToUpper(hex)
		decoded := hexByte(upper)
		if isUnreserved(decoded) {
			b.WriteByte(decoded)
		} else {
			b.WriteByte('%')
			b.WriteString(upper)
		}
		i += 2
	}

	return b.String()
}

func isHexPair(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'A' || c > 'F') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func hexByte(hex string) byte {
	var n byte
	for i := 0; i < 2; i++ {
		n <<= 4
		c := hex[i]
		switch {
		case c >= '0' && c <= '9':
			n |= c - '0'
		case c >= 'A' && c <= 'F':
			n |= c - 'A' + 10
		}
	}
	return n
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '.' || c == '_' || c == '~'
}
