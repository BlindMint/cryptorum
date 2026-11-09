package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cryptorum/internal/scanner"
	"github.com/go-chi/chi/v5"
)

func setupScanJobTest(t *testing.T) {
	setupCombineTestDB(t)
	previousScanner := appScanner
	previousLibraries := scanningLibraries
	previousShutdown := scanShutdown.Load()
	appScanner = scanner.New(appDB.DB, t.TempDir(), t.TempDir())
	scanningLibraries = map[int64]bool{}
	// Tests explicitly execute the worker to simulate stop/restart boundaries.
	scanShutdown.Store(true)
	t.Cleanup(func() {
		appScanner = previousScanner
		scanningLibraries = previousLibraries
		scanShutdown.Store(previousShutdown)
	})
}

func scanControlRequest(t *testing.T, id, action string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/"+id+"/"+action, nil)
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	route := chi.NewRouteContext()
	route.URLParams.Add("jobID", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestScanPauseResumeCheckpointAndDedupe(t *testing.T) {
	setupScanJobTest(t)
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	id, _, _, err := queueLibraryScanWithWorker(1, []string{root}, false)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, `CREATE TRIGGER pause_after_file AFTER INSERT ON scan_file_checkpoint BEGIN UPDATE metadata_job SET status='pausing' WHERE id=NEW.job_id; END`)
	scanShutdown.Store(false)
	processLibraryScanJob(id, 1, "Main", []string{root}, "Scan")
	job, err := loadAdminJob(id)
	if err != nil || job.Status != "paused" || job.CompletedItems != 1 {
		t.Fatalf("paused job %+v, %v", job, err)
	}
	existing, queued, _, err := queueLibraryScanWithWorker(1, []string{root}, false)
	if err != nil || queued || existing != id {
		t.Fatalf("paused job lost reservation: %d %v %v", existing, queued, err)
	}
	mustExec(t, `DROP TRIGGER pause_after_file`)
	scanShutdown.Store(true)
	rec := scanControlRequest(t, "1", "resume", ResumeScanJobHandler)
	if rec.Code != 202 {
		t.Fatalf("resume %d: %s", rec.Code, rec.Body.String())
	}
	scanShutdown.Store(false)
	processLibraryScanJob(id, 1, "Main", []string{root}, "Scan")
	job, err = loadAdminJob(id)
	if err != nil || job.Status != "completed" || job.CompletedItems != 2 {
		t.Fatalf("resumed job %+v, %v", job, err)
	}
	var count int
	if err := appDB.QueryRow(`SELECT count(*) FROM book`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("duplicate books after resume: %d", count)
	}
}

func TestInterruptedScanStateRecovery(t *testing.T) {
	setupScanJobTest(t)
	states := []string{"running", "pausing", "paused", "cancelling", "queued"}
	expected := []string{"queued", "paused", "paused", "cancelled", "queued"}
	for i, state := range states {
		mustExec(t, `INSERT INTO metadata_job(id,job_type,title,status,created_at,completed_items) VALUES(?,'library_scan','Scan',?,1,7)`, i+1, state)
	}
	requeueInterruptedLibraryScans()
	for i, want := range expected {
		job, err := loadAdminJob(int64(i + 1))
		if err != nil || job.Status != want || job.CompletedItems != 7 {
			t.Fatalf("recovery %s: %+v %v", states[i], job, err)
		}
	}
	rec := scanControlRequest(t, "3", "cancel", CancelJobHandler)
	if rec.Code != 202 {
		t.Fatal(rec.Body.String())
	}
	job, _ := loadAdminJob(3)
	if job.Status != "cancelled" {
		t.Fatal(job.Status)
	}
}

func TestShutdownCheckpointsWithoutCancellingJob(t *testing.T) {
	setupScanJobTest(t)
	root := t.TempDir()
	id, _, _, err := queueLibraryScanWithWorker(1, []string{root}, false)
	if err != nil {
		t.Fatal(err)
	}
	processLibraryScanJob(id, 1, "Main", []string{root}, "Scan")
	job, err := loadAdminJob(id)
	if err != nil || job.Status != "queued" || job.CompletedAt != nil {
		t.Fatalf("shutdown job %+v %v", job, err)
	}
}

func TestFileImportDoesNotReconcileAndBookdropWaitsForSuccess(t *testing.T) {
	setupScanJobTest(t)
	root := t.TempDir()
	gone := filepath.Join(root, "gone.txt")
	insertCombineTestBook(t, 10, 1, "Existing", gone, "txt")
	path := filepath.Join(root, "drop.txt")
	mustExec(t, `INSERT INTO bookdrop_file(id,filename,path,status,added_at) VALUES(1,'drop.txt',?,'pending',1)`, path)
	id, err := queueFileImport(1, path, 1)
	if err != nil {
		t.Fatal(err)
	}
	scanShutdown.Store(false)
	processLibraryScanJob(id, 1, "Main", []string{path}, "Import")
	var status string
	appDB.QueryRow(`SELECT status FROM bookdrop_file WHERE id=1`).Scan(&status)
	if status == "imported" {
		t.Fatal("failed import marked imported")
	}
	if err := os.WriteFile(path, []byte("new file"), 0600); err != nil {
		t.Fatal(err)
	}
	scanShutdown.Store(true)
	rec := scanControlRequest(t, "1", "resume", ResumeScanJobHandler)
	if rec.Code != 202 {
		t.Fatalf("resume: %s", rec.Body.String())
	}
	scanShutdown.Store(false)
	processLibraryScanJob(id, 1, "Main", []string{path}, "Import")
	appDB.QueryRow(`SELECT status FROM bookdrop_file WHERE id=1`).Scan(&status)
	if status != "imported" {
		t.Fatal(status)
	}
	var missing bool
	appDB.QueryRow(`SELECT missing_at IS NOT NULL FROM book_file WHERE id=10`).Scan(&missing)
	if missing {
		t.Fatal("file import reconciled unrelated book")
	}
	job, _ := loadAdminJob(id)
	var result map[string]any
	if err := json.Unmarshal(job.Result, &result); err != nil {
		t.Fatal(err)
	}
	if job.Status != "completed" {
		t.Fatalf("result: %s", job.Result)
	}
}

func TestWatcherEventsDuringImportRequestFollowupPass(t *testing.T) {
	setupScanJobTest(t)
	path := filepath.Join(t.TempDir(), "changed.txt")
	if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := queueFileImport(1, path, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := queueFileImport(1, path, 0)
	if err != nil || first != second {
		t.Fatalf("event dedupe %d %d %v", first, second, err)
	}
	scanShutdown.Store(false)
	processLibraryScanJob(first, 1, "Main", []string{path}, "Import")
	job, err := loadAdminJob(first)
	if err != nil || job.Status != "queued" {
		t.Fatalf("lost follow-up pass: %s %v", job.Status, err)
	}
	processLibraryScanJob(first, 1, "Main", []string{path}, "Import")
	job, err = loadAdminJob(first)
	if err != nil || job.Status != "completed" {
		t.Fatalf("follow-up did not complete: %s %v", job.Status, err)
	}
	var count int
	if err := appDB.QueryRow(`SELECT count(*) FROM book`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate book after follow-up: %d", count)
	}
}

func TestWorkerPauseAllowsOtherJobsAndShutdownKeepsPaused(t *testing.T) {
	setupScanJobTest(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("worker test"), 0600); err != nil {
		t.Fatal(err)
	}
	first, _, _, err := queueLibraryScanWithWorker(1, []string{root}, false)
	if err != nil {
		t.Fatal(err)
	}
	second, _, _, err := queueLibraryScanWithWorker(2, []string{t.TempDir()}, false)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, `CREATE TRIGGER worker_pause AFTER INSERT ON scan_file_checkpoint WHEN NEW.job_id=1 BEGIN UPDATE metadata_job SET status='pausing' WHERE id=NEW.job_id; END`)
	t.Cleanup(func() {
		if !waitForScanShutdown(time.Second) {
			t.Error("worker failed to stop")
		}
	})
	scanShutdown.Store(false)
	signalLibraryScanWorker()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a, errA := loadAdminJob(first)
		b, errB := loadAdminJob(second)
		if errA == nil && errB == nil && a.Status == "paused" && b.Status == "completed" {
			if !waitForScanShutdown(time.Second) {
				t.Fatal("worker did not shut down")
			}
			requeueInterruptedLibraryScans()
			a, _ = loadAdminJob(first)
			if a.Status != "paused" {
				t.Fatal("shutdown/recovery resumed paused job")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("worker did not pause and process next queued job")
}
