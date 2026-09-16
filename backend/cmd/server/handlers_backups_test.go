package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cryptorum/internal/config"
)

func TestValidateBackupCron(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		wantErr bool
	}{
		{name: "weekly", spec: "0 4 * * 1"},
		{name: "daily", spec: "0 4 * * *"},
		{name: "steps and ranges", spec: "*/15 8-17 * * 1-5"},
		{name: "invalid weekday", spec: "* * * * 9", wantErr: true},
		{name: "missing field", spec: "0 4 * *", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateBackupCron(test.spec)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateBackupCron(%q) error = %v, wantErr %v", test.spec, err, test.wantErr)
			}
		})
	}
}

func TestUpdateBackupSettingsRejectsInvalidCronBeforeChangingConfig(t *testing.T) {
	previousConfig := appConfig
	appConfig = &config.Config{
		Tasks: config.TasksConfig{
			DatabaseBackup: config.BackupConfig{Enabled: true, Cron: "0 4 * * 1", KeepLast: 14},
		},
	}
	t.Cleanup(func() { appConfig = previousConfig })

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/settings/backups",
		strings.NewReader(`{"enabled":true,"cron":"* * * * 9","keep_last":14}`),
	)
	req = req.WithContext(authContextWithUser(context.Background(), &AppUser{ID: 1}))
	recorder := httptest.NewRecorder()

	updateBackupSettingsHandler(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if appConfig.Tasks.DatabaseBackup.Cron != "0 4 * * 1" {
		t.Fatalf("cron changed to %q after invalid request", appConfig.Tasks.DatabaseBackup.Cron)
	}
	if !strings.Contains(recorder.Body.String(), "Invalid cron schedule") {
		t.Fatalf("response does not explain cron error: %s", recorder.Body.String())
	}
}
