package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupReadingProgressTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE reading_progress (
			id INTEGER PRIMARY KEY,
			book_id INTEGER NOT NULL UNIQUE,
			percent REAL,
			status TEXT NOT NULL DEFAULT 'unread',
			updated_at INTEGER NOT NULL,
			owner_user_id INTEGER NOT NULL DEFAULT 1
		)
	`)
	if err != nil {
		t.Fatalf("create reading_progress: %v", err)
	}

	return db
}

func TestTouchReadingProgressForSessionCreatesReadingProgress(t *testing.T) {
	db := setupReadingProgressTestDB(t)

	if err := touchReadingProgressForSession(db, 12, 7, 100); err != nil {
		t.Fatalf("touch reading progress: %v", err)
	}

	var status string
	var percent float64
	var updatedAt, ownerUserID int64
	if err := db.QueryRow(`
		SELECT status, percent, updated_at, owner_user_id
		FROM reading_progress
		WHERE book_id = 12
	`).Scan(&status, &percent, &updatedAt, &ownerUserID); err != nil {
		t.Fatalf("query reading progress: %v", err)
	}

	if status != "reading" || percent != 0 || updatedAt != 100 || ownerUserID != 7 {
		t.Fatalf("unexpected progress row: status=%q percent=%v updated_at=%d owner_user_id=%d", status, percent, updatedAt, ownerUserID)
	}
}

func TestTouchReadingProgressForSessionRefreshesReadingOrder(t *testing.T) {
	db := setupReadingProgressTestDB(t)

	_, err := db.Exec(`
		INSERT INTO reading_progress (book_id, percent, status, updated_at, owner_user_id)
		VALUES (12, 42, 'unread', 50, 7)
	`)
	if err != nil {
		t.Fatalf("insert reading progress: %v", err)
	}

	if err := touchReadingProgressForSession(db, 12, 7, 200); err != nil {
		t.Fatalf("touch reading progress: %v", err)
	}

	var status string
	var percent float64
	var updatedAt int64
	if err := db.QueryRow(`
		SELECT status, percent, updated_at
		FROM reading_progress
		WHERE book_id = 12
	`).Scan(&status, &percent, &updatedAt); err != nil {
		t.Fatalf("query reading progress: %v", err)
	}

	if status != "reading" || percent != 42 || updatedAt != 200 {
		t.Fatalf("unexpected refreshed progress row: status=%q percent=%v updated_at=%d", status, percent, updatedAt)
	}
}

func TestTouchReadingProgressForSessionPreservesFinishedStatus(t *testing.T) {
	db := setupReadingProgressTestDB(t)

	_, err := db.Exec(`
		INSERT INTO reading_progress (book_id, percent, status, updated_at, owner_user_id)
		VALUES (12, 100, 'finished', 50, 7)
	`)
	if err != nil {
		t.Fatalf("insert reading progress: %v", err)
	}

	if err := touchReadingProgressForSession(db, 12, 7, 200); err != nil {
		t.Fatalf("touch reading progress: %v", err)
	}

	var status string
	var percent float64
	var updatedAt int64
	if err := db.QueryRow(`
		SELECT status, percent, updated_at
		FROM reading_progress
		WHERE book_id = 12
	`).Scan(&status, &percent, &updatedAt); err != nil {
		t.Fatalf("query reading progress: %v", err)
	}

	if status != "finished" || percent != 100 || updatedAt != 200 {
		t.Fatalf("unexpected finished progress row: status=%q percent=%v updated_at=%d", status, percent, updatedAt)
	}
}
