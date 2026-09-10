package crawler

import (
	"strings"
	"testing"
	"time"
)

func TestRetryConfigValidate(t *testing.T) {
	valid := DefaultRetryConfig()

	cases := []struct {
		name    string
		mutate  func(RetryConfig) RetryConfig
		wantErr string
	}{
		{
			name:   "default is valid",
			mutate: func(rc RetryConfig) RetryConfig { return rc },
		},
		{
			name:    "zero value",
			mutate:  func(RetryConfig) RetryConfig { return RetryConfig{} },
			wantErr: "BaseTimeout must be > 0",
		},
		{
			name: "negative MaxRetries",
			mutate: func(rc RetryConfig) RetryConfig {
				rc.MaxRetries = -1
				return rc
			},
			wantErr: "MaxRetries must be >= 0",
		},
		{
			name: "zero BaseTimeout",
			mutate: func(rc RetryConfig) RetryConfig {
				rc.BaseTimeout = 0
				return rc
			},
			wantErr: "BaseTimeout must be > 0",
		},
		{
			name: "TimeoutMultiplier below 1",
			mutate: func(rc RetryConfig) RetryConfig {
				rc.TimeoutMultiplier = 0.5
				return rc
			},
			wantErr: "TimeoutMultiplier must be >= 1",
		},
		{
			name: "negative BaseBackoff",
			mutate: func(rc RetryConfig) RetryConfig {
				rc.BaseBackoff = -time.Second
				return rc
			},
			wantErr: "BaseBackoff must be >= 0",
		},
		{
			name: "ceiling below BaseTimeout",
			mutate: func(rc RetryConfig) RetryConfig {
				rc.MaxTotalPerPage = rc.BaseTimeout / 2
				return rc
			},
			wantErr: "must be >= BaseTimeout",
		},
		{
			name: "MaxRetries zero is allowed",
			mutate: func(rc RetryConfig) RetryConfig {
				rc.MaxRetries = 0
				return rc
			},
		},
		{
			name: "BaseBackoff zero is allowed",
			mutate: func(rc RetryConfig) RetryConfig {
				rc.BaseBackoff = 0
				return rc
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.mutate(valid).Validate()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate: want error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("Validate: error %q does not contain %q", err, c.wantErr)
			}
		})
	}
}
