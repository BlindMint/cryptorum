package auth

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T, duration time.Duration) (*Store, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	_, err = db.Exec(`
		CREATE TABLE app_user (
			id INTEGER PRIMARY KEY,
			username TEXT NOT NULL UNIQUE
		);
		CREATE TABLE auth_session (
			id INTEGER PRIMARY KEY,
			token_hash TEXT NOT NULL UNIQUE,
			user_id INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			last_seen_at INTEGER NOT NULL,
			revoked_at INTEGER,
			FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE
		);
		INSERT INTO app_user (id, username) VALUES (1, 'admin');
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return NewStore(db, duration), db
}

func TestSessionPersistsAcrossStoreInstances(t *testing.T) {
	store, db := newTestStore(t, time.Hour)

	session, err := store.CreateSession(1, "admin")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	restartedStore := NewStore(db, time.Hour)
	validated, err := restartedStore.ValidateSession(session.ID)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if validated == nil {
		t.Fatal("expected session to survive store recreation")
	}
	if validated.UserID != 1 || validated.Username != "admin" {
		t.Fatalf("unexpected session user: %#v", validated)
	}

	var storedToken string
	if err := db.QueryRow("SELECT token_hash FROM auth_session").Scan(&storedToken); err != nil {
		t.Fatalf("read stored token: %v", err)
	}
	if storedToken == session.ID {
		t.Fatal("stored session token should be hashed, not raw")
	}
}

func TestDeleteSessionRevokesSession(t *testing.T) {
	store, _ := newTestStore(t, time.Hour)

	session, err := store.CreateSession(1, "admin")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	store.DeleteSession(session.ID)

	validated, err := store.ValidateSession(session.ID)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if validated != nil {
		t.Fatal("expected deleted session to be invalid")
	}
}

func TestExpiredSessionIsInvalid(t *testing.T) {
	store, db := newTestStore(t, time.Hour)

	session, err := store.CreateSession(1, "admin")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_, err = db.Exec("UPDATE auth_session SET expires_at = ?", time.Now().Add(-time.Minute).Unix())
	if err != nil {
		t.Fatalf("expire session: %v", err)
	}

	validated, err := store.ValidateSession(session.ID)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if validated != nil {
		t.Fatal("expected expired session to be invalid")
	}
}
