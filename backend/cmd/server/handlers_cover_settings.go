package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"cryptorum/internal/covers"
)

type bookCoverSettingsResponse struct {
	PreserveFullCover    bool    `json:"preserve_full_cover"`
	VerticalCropping     bool    `json:"vertical_cropping"`
	HorizontalCropping   bool    `json:"horizontal_cropping"`
	AspectRatioThreshold float64 `json:"aspect_ratio_threshold"`
	SmartCropping        bool    `json:"smart_cropping"`
}

func loadBookCoverSettingsResponse() bookCoverSettingsResponse {
	s := covers.LoadSettings(appDB.DB)
	return bookCoverSettingsResponse{
		PreserveFullCover:    s.PreserveFullCover,
		VerticalCropping:     s.VerticalCropping,
		HorizontalCropping:   s.HorizontalCropping,
		AspectRatioThreshold: s.AspectRatioThreshold,
		SmartCropping:        s.SmartCropping,
	}
}

func updateBookCoverSettingsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req bookCoverSettingsResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	settings := covers.Settings{
		PreserveFullCover:    req.PreserveFullCover,
		VerticalCropping:     req.VerticalCropping,
		HorizontalCropping:   req.HorizontalCropping,
		AspectRatioThreshold: req.AspectRatioThreshold,
		SmartCropping:        req.SmartCropping,
	}

	if settings.AspectRatioThreshold <= 0 {
		settings.AspectRatioThreshold = covers.DefaultSettings().AspectRatioThreshold
	}

	if err := covers.SaveSettings(appDB.DB, settings); err != nil {
		slog.Error("Failed to save book cover settings", "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to save book cover settings")
		return
	}

	recordAppLog("info", "covers", "Saved book cover settings", map[string]any{
		"vertical_cropping":   settings.VerticalCropping,
		"horizontal_cropping": settings.HorizontalCropping,
		"aspect_threshold":    settings.AspectRatioThreshold,
		"smart_cropping":      settings.SmartCropping,
	})
	jsonResponse(w, http.StatusOK, loadBookCoverSettingsResponse())
}

func regenerateBookCoversHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req struct {
		Mode      string                     `json:"mode"`
		LibraryID int64                      `json:"library_id,omitempty"`
		Settings  *bookCoverSettingsResponse `json:"settings,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != "all" && mode != "missing" {
		errorResponse(w, http.StatusBadRequest, "Mode must be 'all' or 'missing'")
		return
	}
	if req.LibraryID < 0 {
		errorResponse(w, http.StatusBadRequest, "Invalid library ID")
		return
	}
	if req.LibraryID > 0 && !userCanAccessAllData(current) {
		var exists bool
		if err := appDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM library WHERE id = ? AND owner_user_id = ?)`, req.LibraryID, current.ID).Scan(&exists); err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to verify library ownership")
			return
		}
		if !exists {
			errorResponse(w, http.StatusForbidden, "Permission denied")
			return
		}
	}

	settings := loadBookCoverSettingsResponse()
	if req.Settings != nil {
		settings = *req.Settings
	}
	if err := covers.SaveSettings(appDB.DB, covers.Settings{
		PreserveFullCover:    settings.PreserveFullCover,
		VerticalCropping:     settings.VerticalCropping,
		HorizontalCropping:   settings.HorizontalCropping,
		AspectRatioThreshold: settings.AspectRatioThreshold,
		SmartCropping:        settings.SmartCropping,
	}); err != nil {
		slog.Error("Failed to persist book cover settings before regeneration", "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to save book cover settings")
		return
	}

	recordAppLog("info", "covers", "Started cover regeneration", map[string]any{
		"mode":       mode,
		"library_id": req.LibraryID,
	})

	missingOnly := mode == "missing"
	total, err := appScanner.CountCoverCandidatesForLibrary(req.LibraryID, missingOnly)
	if err != nil {
		slog.Error("Failed to count cover regeneration candidates", "mode", mode, "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to queue cover regeneration")
		return
	}

	title := "Regenerate book covers"
	if missingOnly {
		title = "Regenerate missing book covers"
	}
	if req.LibraryID > 0 {
		title += fmt.Sprintf(" for library %d", req.LibraryID)
	}
	payload, _ := json.Marshal(map[string]any{"mode": mode, "library_id": req.LibraryID})
	now := time.Now().Unix()
	res, err := appDB.Exec(`
		INSERT INTO metadata_job (
			job_type, title, status, payload_json,
			total_items, completed_items, failed_items,
			created_at
		) VALUES (?, ?, ?, ?, ?, 0, 0, ?)
	`, "cover_regenerate", title, "queued", nullString(payload), total, now)
	if err != nil {
		slog.Error("Failed to queue cover regeneration job", "mode", mode, "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to queue cover regeneration")
		return
	}

	jobID, _ := res.LastInsertId()
	createAdminNotification(
		"job_queued",
		title,
		"Queued a background cover regeneration job.",
		"/settings?tab=admin",
	)
	recordAppLog("info", "jobs", "Queued cover regeneration job", map[string]any{
		"job_id":     jobID,
		"mode":       mode,
		"library_id": req.LibraryID,
		"total":      total,
	})

	go processCoverRegenerationJob(jobID, title, mode, req.LibraryID, missingOnly, total)

	job, err := loadAdminJob(jobID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load queued job")
		return
	}

	jsonResponse(w, http.StatusAccepted, job)
}

func processCoverRegenerationJob(jobID int64, title, mode string, libraryID int64, missingOnly bool, total int) {
	startedAt := time.Now().Unix()
	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, started_at = ?
		WHERE id = ?
	`, "running", startedAt, jobID)

	updated, failed, err := appScanner.RegenerateCoversForLibrary(libraryID, missingOnly, func(processed, updated, failed, total int) {
		_, _ = appDB.Exec(`
			UPDATE metadata_job
			SET completed_items = ?, failed_items = ?
			WHERE id = ?
		`, processed, failed, jobID)
	})
	completedAt := time.Now().Unix()
	if err != nil {
		_, _ = appDB.Exec(`
			UPDATE metadata_job
			SET status = ?, error = ?, completed_at = ?
			WHERE id = ?
		`, "failed", err.Error(), completedAt, jobID)
		slog.Error("Failed to regenerate book covers", "mode", mode, "library_id", libraryID, "error", err)
		recordAppLog("error", "covers", "Cover regeneration failed", map[string]any{
			"job_id":     jobID,
			"mode":       mode,
			"library_id": libraryID,
			"error":      err.Error(),
		})
		createAdminNotification(
			"job_failed",
			title,
			fmt.Sprintf("Cover regeneration failed: %s", err.Error()),
			"/settings?tab=admin",
		)
		return
	}

	resultPayload := map[string]any{
		"mode":       mode,
		"library_id": libraryID,
		"updated":    updated,
		"failed":     failed,
		"total":      total,
	}
	resultJSON, _ := json.Marshal(resultPayload)
	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, completed_items = ?, failed_items = ?, result_json = ?, completed_at = ?
		WHERE id = ?
	`, "completed", total, failed, nullString(resultJSON), completedAt, jobID)

	slog.Info("Book cover regeneration finished", "mode", mode, "library_id", libraryID, "updated", updated, "total", total)
	recordAppLog("info", "covers", "Cover regeneration finished", map[string]any{
		"job_id":     jobID,
		"mode":       mode,
		"library_id": libraryID,
		"updated":    updated,
		"total":      total,
	})
	createAdminNotification(
		"job_completed",
		title,
		fmt.Sprintf("Cover regeneration finished: %d updated, %d failed from %d checked.", updated, failed, total),
		"/settings?tab=admin",
	)
}
