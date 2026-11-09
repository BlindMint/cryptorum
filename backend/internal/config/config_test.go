package config

import (
	"github.com/spf13/viper"
	"os"
	"path/filepath"
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

func TestValidateScanPreparationWorkers(t *testing.T) {
	for _, workers := range []int{-1, 0, 1, 2, 3, 4, 5} {
		cfg := &Config{Auth: AuthConfig{Mode: "none"}, Logging: LoggingConfig{Level: "info"}, Scan: ScanConfig{PreparationWorkers: workers}}
		err := validate(cfg)
		if (workers < 0 || workers > 4) != (err != nil) {
			t.Fatalf("workers %d: %v", workers, err)
		}
		if workers == 0 && cfg.Scan.PreparationWorkers != 2 {
			t.Fatal("missing worker default")
		}
	}
}

func TestLoadScanWorkerDefaultAndOverride(t *testing.T) {
	for _, tc := range []struct {
		name, scan string
		want       int
	}{{"default", "", 2}, {"single", "scan:\n  preparation_workers: 1\n", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			root := t.TempDir()
			path := filepath.Join(root, "config.yaml")
			contents := "auth:\n  mode: none\nserver:\n  data_path: " + filepath.Join(root, "data") + "\nbookdrop:\n  path: " + filepath.Join(root, "bookdrop") + "\n" + tc.scan
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil || cfg.Scan.PreparationWorkers != tc.want {
				t.Fatalf("load workers: %+v %v", cfg, err)
			}
		})
	}
}
