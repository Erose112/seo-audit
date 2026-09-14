package crawler

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
)

// FetchErrorKind classifies why a fetch failed. It drives two separate
// decisions: whether the attempt is worth retrying, and whether the failure is
// page-level (scored normally) or systemic (exit 2).
type FetchErrorKind int

const (
	ErrKindTimeout FetchErrorKind = iota
	ErrKindConnection
	ErrKindTLS
	ErrKindHTTPStatus
	// ErrKindPermanent covers deterministic non-network failures such as DNS
	// NXDOMAIN and redirect loops, where a retry reproduces the same outcome.
	ErrKindPermanent
	// ErrKindCanceled means the root context died. Always systemic.
	ErrKindCanceled
	// ErrKindParseFailure means the page fetched but could not be parsed or was
	// not HTML. Recorded in CrawlResult.Errors; excluded from Rule 3 fail-rate.
	ErrKindParseFailure
)

// ErrSystemicFailureRate is returned when more than half of fetch attempts fail
// after at least five have been tried.
var ErrSystemicFailureRate = errors.New("failure rate exceeds 50%")

// ErrUnknownErrorKind is returned when a FetchErrorKind cannot be decoded from
// its wire representation.
var ErrUnknownErrorKind = errors.New("unknown fetch error kind")

func (k FetchErrorKind) String() string {
	switch k {
	case ErrKindTimeout:
		return "timeout"
	case ErrKindConnection:
		return "connection"
	case ErrKindTLS:
		return "tls"
	case ErrKindHTTPStatus:
		return "http_status"
	case ErrKindPermanent:
		return "permanent"
	case ErrKindCanceled:
		return "canceled"
	case ErrKindParseFailure:
		return "parse_failure"
	}
	return "unknown"
}

func (k FetchErrorKind) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

func (k *FetchErrorKind) UnmarshalText(text []byte) error {
	switch string(text) {
	case "timeout":
		*k = ErrKindTimeout
	case "connection":
		*k = ErrKindConnection
	case "tls":
		*k = ErrKindTLS
	case "http_status":
		*k = ErrKindHTTPStatus
	case "permanent":
		*k = ErrKindPermanent
	case "canceled":
		*k = ErrKindCanceled
	case "parse_failure":
		*k = ErrKindParseFailure
	default:
		return fmt.Errorf("%w: %q", ErrUnknownErrorKind, text)
	}
	return nil
}

// CrawlError records a page-level failure for reporting.
type CrawlError struct {
	URL        string
	Kind       FetchErrorKind
	StatusCode int
	Message    string
}

type FetchError struct {
	Kind       FetchErrorKind
	StatusCode int
	Err        error
}

func (e *FetchError) Error() string {
	if e.Kind == ErrKindHTTPStatus {
		return fmt.Sprintf("%s: %d: %v", e.Kind, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Kind, e.Err)
}

func (e *FetchError) Unwrap() error { return e.Err }

// classifyFetchError maps a transport-level error onto a FetchErrorKind.
//
// The distinction that matters most is deterministic vs. transient: a typo'd
// hostname, an expired certificate, and a redirect loop all fail identically on
// every retry, so lumping them into the retryable connection bucket would spend
// the full ~74s retry schedule to reach a guaranteed-identical result.
func classifyFetchError(err error) error {
	if err == nil {
		return nil
	}

	var already *FetchError
	if errors.As(err, &already) {
		return err
	}

	if errors.Is(err, context.Canceled) {
		return &FetchError{Kind: ErrKindCanceled, Err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &FetchError{Kind: ErrKindTimeout, Err: err}
	}
	if errors.Is(err, ErrTooManyRedirects) {
		return &FetchError{Kind: ErrKindPermanent, Err: err}
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTimeout {
			return &FetchError{Kind: ErrKindTimeout, Err: err}
		}
		return &FetchError{Kind: ErrKindPermanent, Err: err}
	}

	var certErr x509.UnknownAuthorityError
	var certInvalidErr x509.CertificateInvalidError
	var hostnameErr x509.HostnameError
	if errors.As(err, &certErr) || errors.As(err, &certInvalidErr) || errors.As(err, &hostnameErr) {
		return &FetchError{Kind: ErrKindTLS, Err: err}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &FetchError{Kind: ErrKindTimeout, Err: err}
	}

	return &FetchError{Kind: ErrKindConnection, Err: err}
}

func isRetryable(err error) bool {
	var fe *FetchError
	if !errors.As(err, &fe) {
		return false
	}
	switch fe.Kind {
	case ErrKindTimeout, ErrKindConnection:
		return true
	case ErrKindCanceled, ErrKindTLS, ErrKindPermanent:
		return false
	case ErrKindHTTPStatus:
		// 404/403/401/400 are legitimate findings for the audit to report, not
		// transient glitches, so retrying them cannot change the outcome.
		return fe.StatusCode == 429 || fe.StatusCode >= 500
	}
	return false
}
