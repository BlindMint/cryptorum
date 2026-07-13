package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cryptorum/internal/db"

	"github.com/go-chi/chi/v5"
)

func setupReadingPositionHandlerTestDB(t *testing.T) {
	t.Helper()
	previousDB := appDB
	testDB, err := db.New(t.TempDir())
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	appDB = testDB
	t.Cleanup(func() {
		_ = testDB.Close()
		appDB = previousDB
	})

	mustExec(t, `INSERT INTO library (id, name, owner_user_id) VALUES (1, 'Main', 1)`)
	mustExec(t, `
		INSERT INTO book (id, library_id, added_at, last_scanned, owner_user_id)
		VALUES (1, 1, 100, 100, 1)
	`)
	mustExec(t, `
		INSERT INTO book_file (id, book_id, path, format, size, hash, last_modified, owner_user_id)
		VALUES (10, 1, 'test.pdf', 'pdf', 1000, 'hash-one', 100, 1),
		       (11, 1, 'test.epub', 'epub', 1000, 'hash-two', 100, 1)
	`)
}

func readingPositionRequest(method, path, body string, params map[string]string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	routeCtx := chi.NewRouteContext()
	for key, value := range params {
		routeCtx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func startPositionSession(t *testing.T, fileID int64, channel, mode string) (int64, readingPosition) {
	t.Helper()
	recorder := httptest.NewRecorder()
	body := fmt.Sprintf(`{"file_id":%d,"channel":%q,"reader_mode":%q}`, fileID, channel, mode)
	req := readingPositionRequest(http.MethodPost, "/api/books/1/reading-sessions", body, map[string]string{"bookID": "1"})
	StartReadingPositionSessionHandler(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("start session status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Session struct {
			ID int64 `json:"id"`
		} `json:"session"`
		Position readingPosition `json:"position"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	return response.Session.ID, response.Position
}

func savePosition(t *testing.T, sessionID, sequence, revision int64, percent float64, reachedEnd bool) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	body := fmt.Sprintf(`{
		"client_sequence":%d,
		"base_revision":%d,
		"reader_mode":"pdf",
		"percent":%f,
		"locator":{"type":"pdf_page","page":228,"total_pages":736},
		"source_hash":"hash-one",
		"reached_end":%t
	}`, sequence, revision, percent, reachedEnd)
	req := readingPositionRequest(
		http.MethodPut,
		fmt.Sprintf("/api/books/1/reading-sessions/%d/position", sessionID),
		body,
		map[string]string{"bookID": "1", "sessionID": fmt.Sprint(sessionID)},
	)
	SaveReadingPositionHandler(recorder, req)
	return recorder
}

func TestReadingPositionSessionMigratesLegacyProgressToExplicitFile(t *testing.T) {
	setupReadingPositionHandlerTestDB(t)
	mustExec(t, `
		INSERT INTO reading_progress (book_id, percent, page, status, updated_at, owner_user_id)
		VALUES (1, 31, 228, 'reading', 100, 1)
	`)

	_, position := startPositionSession(t, 10, "standard", "pdf")
	if position.FileID != 10 || position.Percent != 31 || position.Revision != 1 {
		t.Fatalf("unexpected migrated position: %+v", position)
	}
	locator := position.Locators["pdf"]
	if locator.Revision != position.Revision || !strings.Contains(string(locator.Locator), `"page":228`) {
		t.Fatalf("legacy page locator was not adopted: %+v", locator)
	}
}

func TestReadingPositionRejectsSupersededAndOutOfOrderSessions(t *testing.T) {
	setupReadingPositionHandlerTestDB(t)
	firstSession, _ := startPositionSession(t, 10, "standard", "pdf")

	firstSave := savePosition(t, firstSession, 1, 0, 31, false)
	if firstSave.Code != http.StatusOK {
		t.Fatalf("first save status = %d: %s", firstSave.Code, firstSave.Body.String())
	}

	duplicate := savePosition(t, firstSession, 1, 1, 31, false)
	if duplicate.Code != http.StatusOK {
		t.Fatalf("idempotent retry status = %d: %s", duplicate.Code, duplicate.Body.String())
	}

	secondSession, position := startPositionSession(t, 10, "standard", "pdf")
	if position.Revision != 1 {
		t.Fatalf("new session revision = %d, want 1", position.Revision)
	}
	staleSave := savePosition(t, firstSession, 2, 1, 32, false)
	if staleSave.Code != http.StatusConflict || !strings.Contains(staleSave.Body.String(), "session_superseded") {
		t.Fatalf("stale save = %d: %s", staleSave.Code, staleSave.Body.String())
	}

	newSave := savePosition(t, secondSession, 1, 1, 32, false)
	if newSave.Code != http.StatusOK {
		t.Fatalf("new authoritative save = %d: %s", newSave.Code, newSave.Body.String())
	}
	olderSequence := savePosition(t, secondSession, 0, 2, 33, false)
	if olderSequence.Code != http.StatusBadRequest {
		t.Fatalf("invalid sequence status = %d, want 400", olderSequence.Code)
	}
}

func TestFinishedPersistsAndUnreadClearsEveryPosition(t *testing.T) {
	setupReadingPositionHandlerTestDB(t)
	sessionID, _ := startPositionSession(t, 10, "standard", "pdf")
	finished := savePosition(t, sessionID, 1, 0, 100, true)
	if finished.Code != http.StatusOK {
		t.Fatalf("finish save status = %d: %s", finished.Code, finished.Body.String())
	}
	backtrack := savePosition(t, sessionID, 2, 1, 31, false)
	if backtrack.Code != http.StatusOK {
		t.Fatalf("backtrack save status = %d: %s", backtrack.Code, backtrack.Body.String())
	}

	var status string
	if err := appDB.QueryRow(`SELECT status FROM reading_progress WHERE book_id = 1 AND owner_user_id = 1`).Scan(&status); err != nil {
		t.Fatalf("load status: %v", err)
	}
	if status != "finished" {
		t.Fatalf("status = %q, want finished", status)
	}

	mustExec(t, `UPDATE reading_progress SET status = 'unread' WHERE book_id = 1 AND owner_user_id = 1`)
	var count int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM reading_position WHERE book_id = 1 AND owner_user_id = 1`).Scan(&count); err != nil {
		t.Fatalf("count positions: %v", err)
	}
	if count != 0 {
		t.Fatalf("positions after unread = %d, want 0", count)
	}
	var percent, speedPercent float64
	var fileID any
	if err := appDB.QueryRow(`
		SELECT percent, COALESCE(speed_reader_percent, 0), file_id
		FROM reading_progress WHERE book_id = 1 AND owner_user_id = 1
	`).Scan(&percent, &speedPercent, &fileID); err != nil {
		t.Fatalf("load reset summary: %v", err)
	}
	if percent != 0 || speedPercent != 0 || fileID != nil {
		t.Fatalf("reset summary percent=%v speed=%v file=%v", percent, speedPercent, fileID)
	}
}
