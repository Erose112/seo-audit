package regression

import (
	"strings"
	"testing"
)

func TestNormalizeRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr string
	}{
		{
			name: "lowercase and whitespace",
			in:   []string{" canonical ", "title_exists"},
			want: []string{"CANONICAL", "TITLE_EXISTS"},
		},
		{
			name: "empty string from cobra opt-out",
			in:   []string{""},
			want: nil,
		},
		{
			name:    "unknown id",
			in:      []string{"CANONICAL_TAG"},
			wantErr: "unknown strict rule",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeRules(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NormalizeRules() error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeRules() unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("NormalizeRules() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("NormalizeRules()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestDefaultStrictRulesValidate(t *testing.T) {
	t.Parallel()
	if _, err := NormalizeRules(DefaultStrictRules()); err != nil {
		t.Fatalf("DefaultStrictRules() failed validation: %v", err)
	}
}
