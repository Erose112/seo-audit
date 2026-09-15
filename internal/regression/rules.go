package regression

import (
	"fmt"
	"strings"

	"github.com/Erose112/seo-audit/internal/checks"
)

// NormalizeRules trims, upper-cases, and drops empty entries, then rejects any
// ID that is not a registered check or a site-level finding.
func NormalizeRules(rules []string) ([]string, error) {
	valid := validCheckIDs()
	var out []string
	for _, rule := range rules {
		id := strings.ToUpper(strings.TrimSpace(rule))
		if id == "" {
			continue
		}
		if _, ok := valid[id]; !ok {
			return nil, fmt.Errorf("regression: unknown strict rule %q", rule)
		}
		out = append(out, id)
	}
	return out, nil
}

func validCheckIDs() map[string]struct{} {
	out := make(map[string]struct{})
	for _, c := range checks.AllChecks() {
		out[c.ID()] = struct{}{}
	}
	out[CheckIDDuplicateTitle] = struct{}{}
	out[CheckIDBrokenLink] = struct{}{}
	return out
}
