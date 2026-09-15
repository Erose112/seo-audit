package buildinfo

import (
	"strings"
	"testing"
)

func TestVersionNonEmpty(t *testing.T) {
	if Version() == "" {
		t.Fatal("Version() is empty")
	}
}

func TestStringFormat(t *testing.T) {
	s := String()
	if !strings.Contains(s, "(") || !strings.Contains(s, ")") {
		t.Fatalf("String() = %q, want parenthesized commit and date", s)
	}
}
