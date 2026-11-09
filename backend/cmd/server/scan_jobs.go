package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
)

var scanShutdown atomic.Bool
var scanWorkerMu sync.Mutex
var scanWorkerWait sync.WaitGroup

// Returns after the current file commits, or at the deadline. A forced exit
// leaves a durable running job and rolls back any unfinished SQLite transaction.
func waitForScanShutdown(timeout time.Duration) bool {
	scanWorkerMu.Lock()
	scanShutdown.Store(true)
	scanWorkerMu.Unlock()
	done := make(chan struct{})
	go func() { scanWorkerWait.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func PauseScanJobHandler(w http.ResponseWriter, r *http.Request)  { changeScanJobState(w, r, "pause") }
func ResumeScanJobHandler(w http.ResponseWriter, r *http.Request) { changeScanJobState(w, r, "resume") }

func changeScanJobState(w http.ResponseWriter, r *http.Request, action string) {
	if !requirePermission(getUserFromContext(r.Context()), PermissionManageJobs) {
		errorResponse(w, 403, "Permission denied")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "jobID"), 10, 64)
	if err != nil {
		errorResponse(w, 400, "Invalid job ID")
		return
	}
	var query string
	if action == "pause" {
		query = `UPDATE metadata_job SET status=CASE WHEN status='queued' THEN 'paused' ELSE 'pausing' END WHERE id=? AND job_type='library_scan' AND status IN ('queued','running')`
	} else {
		query = `UPDATE metadata_job SET status='queued',cancel_requested_at=NULL,completed_at=NULL,error=NULL WHERE id=? AND job_type='library_scan' AND status IN ('paused','failed','cancelled')`
	}
	result, err := appDB.Exec(query, id)
	if err != nil {
		errorResponse(w, 409, "Unable to change this scan; another scan for this library may already be active")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		errorResponse(w, 409, "This scan cannot perform that action in its current state")
		return
	}
	if action == "resume" {
		signalLibraryScanWorker()
	}
	recordAppLog("info", "jobs", "Scan "+action+" requested", map[string]any{"job_id": id})
	jsonResponse(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// File imports have their own dedupe key and never perform missing-file reconciliation.
// They run in the same serial worker as full scans.
func queueFileImport(libraryID int64, path string, bookdropID int64) (int64, error) {
	payload, _ := json.Marshal(map[string]any{"library_id": libraryID, "paths": []string{path}, "partial": true, "bookdrop_id": bookdropID})
	key := fmt.Sprintf("library_scan:file:%d:%s:%d", libraryID, path, bookdropID)
	tx, err := appDB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT OR IGNORE INTO metadata_job(job_type,title,status,payload_json,created_at,dedupe_key) VALUES('library_scan',?,'queued',?,?,?)`, "Import file: "+path, string(payload), time.Now().Unix(), key)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	var id int64
	if n > 0 {
		id, _ = result.LastInsertId()
	} else {
		err = tx.QueryRow(`SELECT id FROM metadata_job WHERE dedupe_key=? AND status IN ('queued','running','pausing','paused','cancelling')`, key).Scan(&id)
		if err != nil {
			return 0, err
		}
		// A watcher event during processing must not be lost to deduplication.
		_, err = tx.Exec(`UPDATE metadata_job SET rescan_requested=1 WHERE id=?`, id)
		if err != nil {
			return 0, err
		}
	}
	err = tx.Commit()

	if err == nil {
		signalLibraryScanWorker()
	}
	return id, err
}

func configuredLibraryPaths(libraryID int64) ([]string, error) {
	rows, err := appDB.Query(`SELECT path FROM library_path WHERE library_id=? ORDER BY id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}
