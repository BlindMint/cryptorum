package config

import (
	"strings"
	"testing"
)

func TestValidateRequiresExplicitAuthMode(t *testing.T) {
	cfg := &Config{Logging: LoggingConfig{Level: "info"}}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "auth.mode must be explicitly set") {
		t.Fatalf("validate() error = %v, want explicit auth mode error", err)
	}
}

func TestValidateRejectsRemovedTrustedNetworkMode(t *testing.T) {
	cfg := &Config{
		Auth:    AuthConfig{Mode: "trusted_network"},
		Logging: LoggingConfig{Level: "info"},
	}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid auth mode") {
		t.Fatalf("validate() error = %v, want invalid auth mode error", err)
	}
}

func TestValidateAcceptsSingleUserAuthModes(t *testing.T) {
	tests := []Config{
		{
			Auth:    AuthConfig{Mode: "none"},
			Logging: LoggingConfig{Level: "info"},
		},
		{
			Auth: AuthConfig{
				Mode:         "password",
				Username:     "reader",
				PasswordHash: "$2a$10$example",
			},
			Logging: LoggingConfig{Level: "info"},
		},
	}

	for _, cfg := range tests {
		if err := validate(&cfg); err != nil {
			t.Fatalf("validate(%q) error = %v", cfg.Auth.Mode, err)
		}
	}
}
