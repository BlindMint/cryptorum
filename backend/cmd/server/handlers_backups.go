package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"

	"cryptorum/internal/config"
	"cryptorum/internal/db"
	"cryptorum/internal/scanner"
)

type BackupSettingsResponse struct {
	Cron     string `json:"cron"`
	KeepLast int    `json:"keep_last"`
}

type BackupItem struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ModifiedAt  int64  `json:"modified_at"`
	DownloadURL string `json:"download_url"`
	RestoreURL  string `json:"restore_url"`
	DeleteURL   string `json:"delete_url"`
}

type BackupListResponse struct {
	Settings BackupSettingsResponse `json:"settings"`
	Items    []BackupItem           `json:"items"`
}

func currentBackupSettings() BackupSettingsResponse {
	return BackupSettingsResponse{
		Cron:     appConfig.Tasks.DatabaseBackup.Cron,
		KeepLast: appConfig.Tasks.DatabaseBackup.KeepLast,
	}
}

func updateBackupSettingsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionCreateBackups) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req BackupSettingsResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	cronValue := strings.TrimSpace(req.Cron)
	keepLast := req.KeepLast
	if keepLast <= 0 {
		keepLast = appConfig.Tasks.DatabaseBackup.KeepLast
		if keepLast <= 0 {
			keepLast = 14
		}
	}

	if err := config.UpdateDatabaseBackupConfig(cronValue, keepLast); err != nil {
		slog.Error("Failed to persist database backup settings", "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to save backup settings")
		return
	}

	appConfig.Tasks.DatabaseBackup.Cron = cronValue
	appConfig.Tasks.DatabaseBackup.KeepLast = keepLast
	startDatabaseBackupSchedule()

	recordAppLog("info", "backup", "Updated backup schedule", map[string]any{
		"cron":      cronValue,
		"keep_last": keepLast,
	})

	jsonResponse(w, http.StatusOK, currentBackupSettings())
}

func startDatabaseBackupSchedule() {
	backupCronMu.Lock()
	defer backupCronMu.Unlock()

	if backupCronRunner != nil {
		backupCronRunner.Stop()
		backupCronRunner = nil
	}

	cronSpec := strings.TrimSpace(appConfig.Tasks.DatabaseBackup.Cron)
	if cronSpec == "" {
		return
	}

	runner := cron.New()
	_, err := runner.AddFunc(cronSpec, func() {
		if _, err := queueDatabaseBackupJob("scheduled"); err != nil {
			slog.Error("Failed to queue scheduled database backup", "error", err)
		}
	})
	if err != nil {
		slog.Error("Failed to schedule database backup", "error", err)
		return
	}

	backupCronRunner = runner
	runner.Start()
}

func stopDatabaseBackupSchedule() {
	backupCronMu.Lock()
	defer backupCronMu.Unlock()

	if backupCronRunner != nil {
		backupCronRunner.Stop()
		backupCronRunner = nil
	}
}

func ListBackupsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionViewAdmin) && !requirePermission(current, PermissionCreateBackups) && !requirePermission(current, PermissionRestoreBackups) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	items, err := listBackupItems()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load backups")
		return
	}

	jsonResponse(w, http.StatusOK, BackupListResponse{
		Settings: currentBackupSettings(),
		Items:    items,
	})
}

func CreateBackupHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionCreateBackups) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	job, err := queueDatabaseBackupJob("manual")
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to queue backup")
		return
	}

	jsonResponse(w, http.StatusAccepted, job)
}

func RestoreBackupHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionRestoreBackups) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	backupName := chi.URLParam(r, "backupName")
	backupPath, err := resolveBackupPath(backupName)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := os.Stat(backupPath); err != nil {
		if os.IsNotExist(err) {
			errorResponse(w, http.StatusNotFound, "Backup not found")
			return
		}
		errorResponse(w, http.StatusInternalServerError, "Failed to inspect backup")
		return
	}

	if maintenanceMode.Swap(true) {
		errorResponse(w, http.StatusConflict, "Database maintenance already in progress")
		return
	}

	defer maintenanceMode.Store(false)

	slog.Info("Starting backup restore", "backup", backupName)

	oldDB := appDB
	if oldDB != nil {
		_ = oldDB.Close()
	}

	if err := replaceDatabaseFile(backupPath, appConfig.GetDBPath()); err != nil {
		slog.Error("Failed to replace database file", "backup", backupName, "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to restore backup")
		return
	}

	newDB, err := db.New(appConfig.Server.DataPath)
	if err != nil {
		slog.Error("Failed to reopen database after restore", "backup", backupName, "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to reopen database after restore")
		return
	}

	appDB = newDB
	appScanner = scanner.New(appDB.DB, appConfig.Server.DataPath, appConfig.GetCoversPath())

	if _, err := ensureBootstrapUser(); err != nil {
		slog.Warn("Failed to re-ensure bootstrap user after restore", "error", err)
	}
	if err := ensureBootstrapOwnership(); err != nil {
		slog.Warn("Failed to re-ensure bootstrap ownership after restore", "error", err)
	}

	recordAppLog("info", "backup", "Restored database backup", map[string]any{
		"backup": backupName,
	})
	createAdminNotification(
		"backup_restored",
		"Database restored",
		fmt.Sprintf("Restored backup %s.", backupName),
		"/settings?tab=admin",
	)

	jsonResponse(w, http.StatusOK, map[string]string{
		"status": "restored",
		"backup": backupName,
	})
}

func DeleteBackupHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionCreateBackups) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	backupName := chi.URLParam(r, "backupName")
	backupPath, err := resolveBackupPath(backupName)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := os.Remove(backupPath); err != nil {
		if os.IsNotExist(err) {
			errorResponse(w, http.StatusNotFound, "Backup not found")
			return
		}
		errorResponse(w, http.StatusInternalServerError, "Failed to delete backup")
		return
	}

	recordAppLog("info", "backup", "Deleted backup file", map[string]any{
		"backup": backupName,
	})

	jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func DownloadBackupHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionViewAdmin) && !requirePermission(current, PermissionCreateBackups) && !requirePermission(current, PermissionRestoreBackups) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	backupName := chi.URLParam(r, "backupName")
	backupPath, err := resolveBackupPath(backupName)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := os.Stat(backupPath); err != nil {
		if os.IsNotExist(err) {
			errorResponse(w, http.StatusNotFound, "Backup not found")
			return
		}
		errorResponse(w, http.StatusInternalServerError, "Failed to open backup")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(backupName)))
	http.ServeFile(w, r, backupPath)
}

func queueDatabaseBackupJob(trigger string) (*AdminJob, error) {
	if appDB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	title := "Database backup"
	payload, _ := json.Marshal(map[string]any{
		"trigger": trigger,
	})
	now := time.Now().Unix()

	res, err := appDB.Exec(`
		INSERT INTO metadata_job (
			job_type, title, status, payload_json,
			total_items, completed_items, failed_items,
			created_at
		) VALUES (?, ?, ?, ?, 1, 0, 0, ?)
	`, "database_backup", title, "queued", nullString(payload), now)
	if err != nil {
		return nil, err
	}

	jobID, _ := res.LastInsertId()
	createAdminNotification(
		"backup_queued",
		title,
		"Queued a background database backup.",
		"/settings?tab=admin",
	)
	recordAppLog("info", "backup", "Queued database backup", map[string]any{
		"job_id":  jobID,
		"trigger": trigger,
	})

	go processDatabaseBackupJob(jobID, trigger, title)

	job, err := loadAdminJob(jobID)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func processDatabaseBackupJob(jobID int64, trigger, title string) {
	startedAt := time.Now().Unix()
	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, started_at = ?
		WHERE id = ?
	`, "running", startedAt, jobID)

	if err := os.MkdirAll(appConfig.GetBackupsPath(), 0755); err != nil {
		markDatabaseBackupJobFailed(jobID, title, trigger, err)
		return
	}

	filename := fmt.Sprintf("cryptorum-%s.db", time.Now().Format("20060102-150405"))
	backupPath := filepath.Join(appConfig.GetBackupsPath(), filename)
	if err := appDB.Backup(backupPath); err != nil {
		markDatabaseBackupJobFailed(jobID, title, trigger, err)
		return
	}

	if err := pruneBackupFiles(appConfig.Tasks.DatabaseBackup.KeepLast); err != nil {
		slog.Warn("Failed to prune old backups", "error", err)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		markDatabaseBackupJobFailed(jobID, title, trigger, err)
		return
	}

	resultPayload := map[string]any{
		"backup": map[string]any{
			"name":         filename,
			"size":         info.Size(),
			"modified_at":  info.ModTime().Unix(),
			"download_url": fmt.Sprintf("/api/backups/%s/download", filename),
			"restore_url":  fmt.Sprintf("/api/backups/%s/restore", filename),
			"delete_url":   fmt.Sprintf("/api/backups/%s", filename),
		},
		"trigger": trigger,
	}
	resultJSON, _ := json.Marshal(resultPayload)
	completedAt := time.Now().Unix()

	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, result_json = ?, completed_items = 1, completed_at = ?
		WHERE id = ?
	`, "completed", nullString(resultJSON), completedAt, jobID)

	createAdminNotification(
		"backup_completed",
		title,
		fmt.Sprintf("Database backup completed: %s", filename),
		"/settings?tab=admin",
	)
	recordAppLog("info", "backup", "Completed database backup", map[string]any{
		"job_id": jobID,
		"backup": filename,
		"size":   info.Size(),
	})
}

func markDatabaseBackupJobFailed(jobID int64, title, trigger string, err error) {
	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, error = ?, completed_at = ?, failed_items = 1
		WHERE id = ?
	`, "failed", err.Error(), time.Now().Unix(), jobID)
	createAdminNotification(
		"backup_failed",
		title,
		fmt.Sprintf("Database backup failed: %v", err),
		"/settings?tab=admin",
	)
	recordAppLog("error", "backup", "Database backup failed", map[string]any{
		"job_id":  jobID,
		"trigger": trigger,
		"error":   err.Error(),
	})
}

func listBackupItems() ([]BackupItem, error) {
	dir := appConfig.GetBackupsPath()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupItem{}, nil
		}
		return nil, err
	}

	items := make([]BackupItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".db") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, BackupItem{
			Name:        name,
			Size:        info.Size(),
			ModifiedAt:  info.ModTime().Unix(),
			DownloadURL: fmt.Sprintf("/api/backups/%s/download", name),
			RestoreURL:  fmt.Sprintf("/api/backups/%s/restore", name),
			DeleteURL:   fmt.Sprintf("/api/backups/%s", name),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].ModifiedAt == items[j].ModifiedAt {
			return items[i].Name > items[j].Name
		}
		return items[i].ModifiedAt > items[j].ModifiedAt
	})

	return items, nil
}

func resolveBackupPath(name string) (string, error) {
	cleanName := filepath.Base(strings.TrimSpace(name))
	if cleanName == "" || cleanName == "." || cleanName != strings.TrimSpace(name) {
		return "", fmt.Errorf("invalid backup name")
	}
	if !strings.HasSuffix(strings.ToLower(cleanName), ".db") {
		return "", fmt.Errorf("invalid backup name")
	}

	baseDir := filepath.Clean(appConfig.GetBackupsPath())
	fullPath := filepath.Clean(filepath.Join(baseDir, cleanName))
	if !strings.HasPrefix(fullPath, baseDir+string(os.PathSeparator)) && fullPath != baseDir {
		return "", fmt.Errorf("invalid backup path")
	}

	return fullPath, nil
}

func replaceDatabaseFile(sourcePath, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		return err
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(destinationPath + suffix)
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	tmpPath := destinationPath + ".restore.tmp"
	target, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := target.Sync(); err != nil {
		target.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, destinationPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func pruneBackupFiles(keepLast int) error {
	if keepLast <= 0 {
		return nil
	}

	items, err := listBackupItems()
	if err != nil {
		return err
	}
	if len(items) <= keepLast {
		return nil
	}

	for _, item := range items[keepLast:] {
		backupPath, err := resolveBackupPath(item.Name)
		if err != nil {
			continue
		}
		_ = os.Remove(backupPath)
	}

	return nil
}
