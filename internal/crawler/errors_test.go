package crawler

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"testing"
)

func TestClassifyFetchError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want FetchErrorKind
	}{
		{"context canceled", context.Canceled, ErrKindCanceled},
		{"deadline exceeded", context.DeadlineExceeded, ErrKindTimeout},
		{
			name: "wrapped deadline from the http client",
			err:  &url.Error{Op: "Get", URL: "https://example.com", Err: context.DeadlineExceeded},
			want: ErrKindTimeout,
		},
		{
			name: "redirect loop sentinel",
			err:  &url.Error{Op: "Get", URL: "https://example.com", Err: ErrTooManyRedirects},
			want: ErrKindPermanent,
		},
		{
			name: "dns not found",
			err:  &net.DNSError{Err: "no such host", Name: "nope.invalid", IsNotFound: true},
			want: ErrKindPermanent,
		},
		{
			name: "dns timeout",
			err:  &net.DNSError{Err: "i/o timeout", Name: "slow.invalid", IsTimeout: true},
			want: ErrKindTimeout,
		},
		{
			name: "unknown certificate authority",
			err:  x509.UnknownAuthorityError{},
			want: ErrKindTLS,
		},
		{
			name: "certificate hostname mismatch",
			err:  x509.HostnameError{Host: "example.com"},
			want: ErrKindTLS,
		},
		{
			name: "connection refused",
			err:  errors.New("connect: connection refused"),
			want: ErrKindConnection,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var fe *FetchError
			if !errors.As(classifyFetchError(c.err), &fe) {
				t.Fatalf("classifyFetchError(%v) did not produce a *FetchError", c.err)
			}
			if fe.Kind != c.want {
				t.Errorf("Kind = %v, want %v", fe.Kind, c.want)
			}
		})
	}
}

func TestClassifyFetchErrorPassthrough(t *testing.T) {
	if got := classifyFetchError(nil); got != nil {
		t.Errorf("classifyFetchError(nil) = %v, want nil", got)
	}

	// An already-classified error keeps its kind rather than being downgraded
	// to the generic connection bucket on a second pass.
	original := &FetchError{Kind: ErrKindHTTPStatus, StatusCode: 503, Err: errors.New("boom")}
	if got := classifyFetchError(original); got != error(original) {
		t.Errorf("classifyFetchError returned %v, want the original error unchanged", got)
	}
}

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"timeout", &FetchError{Kind: ErrKindTimeout}, true},
		{"connection", &FetchError{Kind: ErrKindConnection}, true},
		{"canceled", &FetchError{Kind: ErrKindCanceled}, false},
		{"tls", &FetchError{Kind: ErrKindTLS}, false},
		{"permanent", &FetchError{Kind: ErrKindPermanent}, false},
		{"429 too many requests", &FetchError{Kind: ErrKindHTTPStatus, StatusCode: 429}, true},
		{"500 server error", &FetchError{Kind: ErrKindHTTPStatus, StatusCode: 500}, true},
		{"503 unavailable", &FetchError{Kind: ErrKindHTTPStatus, StatusCode: 503}, true},
		{"404 not found", &FetchError{Kind: ErrKindHTTPStatus, StatusCode: 404}, false},
		{"403 forbidden", &FetchError{Kind: ErrKindHTTPStatus, StatusCode: 403}, false},
		{"401 unauthorized", &FetchError{Kind: ErrKindHTTPStatus, StatusCode: 401}, false},
		{"unclassified error", errors.New("boom"), false},
		{"wrapped retryable", errors.Join(errors.New("context"), &FetchError{Kind: ErrKindTimeout}), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isRetryable(c.err); got != c.want {
				t.Errorf("isRetryable(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
