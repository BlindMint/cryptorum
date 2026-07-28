package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cryptorum/internal/auth"
	"cryptorum/internal/config"
	cryptorumdb "cryptorum/internal/db"

	_ "modernc.org/sqlite"
)

func setupSingleUserAuthTest(t *testing.T) {
	t.Helper()

	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE app_user (
			id INTEGER PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL DEFAULT '',
			is_admin INTEGER NOT NULL DEFAULT 0,
			is_bootstrap_admin INTEGER NOT NULL DEFAULT 0,
			permissions_json TEXT NOT NULL DEFAULT '[]',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
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
	`); err != nil {
		t.Fatalf("create auth schema: %v", err)
	}

	ownerHash, err := auth.HashPassword("owner-password")
	if err != nil {
		t.Fatalf("hash owner password: %v", err)
	}
	legacyHash, err := auth.HashPassword("legacy-password")
	if err != nil {
		t.Fatalf("hash legacy password: %v", err)
	}
	now := time.Now().Unix()
	if _, err := conn.Exec(`
		INSERT INTO app_user (
			id, username, password_hash, is_admin, is_bootstrap_admin,
			permissions_json, created_at, updated_at
		) VALUES
			(1, 'owner', ?, 1, 1, '[]', ?, ?),
			(2, 'legacy', ?, 0, 0, '[]', ?, ?)
	`, ownerHash, now, now, legacyHash, now, now); err != nil {
		t.Fatalf("insert users: %v", err)
	}

	previousConfig := appConfig
	previousDB := appDB
	previousStore := sessionStore
	appConfig = &config.Config{
		Auth: config.AuthConfig{
			Mode:            "password",
			Username:        "owner",
			PasswordHash:    ownerHash,
			SessionDuration: time.Hour,
		},
	}
	appDB = &cryptorumdb.DB{DB: conn}
	sessionStore = auth.NewStore(conn, time.Hour)
	maintenanceMode.Store(false)
	loginThrottle.Lock()
	loginThrottle.entries = make(map[string]loginThrottleEntry)
	loginThrottle.Unlock()

	t.Cleanup(func() {
		appConfig = previousConfig
		appDB = previousDB
		sessionStore = previousStore
		_ = conn.Close()
	})
}

func TestLoginAcceptsOnlyConfiguredSingleUser(t *testing.T) {
	setupSingleUserAuthTest(t)

	legacyBody := `{"username":"legacy","password":"legacy-password"}`
	legacyRec := httptest.NewRecorder()
	loginHandler(legacyRec, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(legacyBody)))
	if legacyRec.Code != http.StatusUnauthorized {
		t.Fatalf("legacy login status = %d, want 401: %s", legacyRec.Code, legacyRec.Body.String())
	}

	ownerBody := `{"username":"owner","password":"owner-password"}`
	ownerRec := httptest.NewRecorder()
	loginHandler(ownerRec, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(ownerBody)))
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner login status = %d, want 200: %s", ownerRec.Code, ownerRec.Body.String())
	}
	if len(ownerRec.Result().Cookies()) == 0 {
		t.Fatal("owner login did not set a session cookie")
	}
}

func TestAuthMiddlewareRejectsLegacySecondaryUserSession(t *testing.T) {
	setupSingleUserAuthTest(t)

	session, err := sessionStore.CreateSession(2, "legacy")
	if err != nil {
		t.Fatalf("create legacy session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()
	authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("legacy session status = %d, want 401", rec.Code)
	}
}

func TestAuthCheckDescribesDisabledSingleUserMode(t *testing.T) {
	setupSingleUserAuthTest(t)
	appConfig.Auth.Mode = "none"

	rec := httptest.NewRecorder()
	authCheckHandler(rec, httptest.NewRequest(http.MethodGet, "/api/auth/check", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("auth check status = %d, want 200", rec.Code)
	}

	var response struct {
		Authenticated bool   `json:"authenticated"`
		AuthDisabled  bool   `json:"auth_disabled"`
		AuthMode      string `json:"auth_mode"`
		UserID        int64  `json:"user_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode auth check: %v", err)
	}
	if !response.Authenticated || !response.AuthDisabled || response.AuthMode != "none" || response.UserID != 1 {
		t.Fatalf("unexpected auth check response: %+v", response)
	}
}

func TestConfiguredCredentialChangeRevokesExistingSession(t *testing.T) {
	setupSingleUserAuthTest(t)

	session, err := sessionStore.CreateSession(1, "owner")
	if err != nil {
		t.Fatalf("create owner session: %v", err)
	}
	newHash, err := auth.HashPassword("new-password")
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}
	appConfig.Auth.PasswordHash = newHash

	if _, err := ensureBootstrapUser(); err != nil {
		t.Fatalf("refresh bootstrap user: %v", err)
	}
	validated, err := sessionStore.ValidateSession(session.ID)
	if err != nil {
		t.Fatalf("validate old session: %v", err)
	}
	if validated != nil {
		t.Fatal("old session remained valid after configured password changed")
	}
}

func TestSameOriginMutationMiddlewareRejectsForeignBrowserOrigin(t *testing.T) {
	handler := sameOriginMutationMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	foreignReq := httptest.NewRequest(http.MethodPost, "https://cryptorum.example/api/settings", nil)
	foreignReq.Header.Set("Origin", "https://attacker.example")
	foreignRec := httptest.NewRecorder()
	handler.ServeHTTP(foreignRec, foreignReq)
	if foreignRec.Code != http.StatusForbidden {
		t.Fatalf("foreign origin status = %d, want 403", foreignRec.Code)
	}

	sameReq := httptest.NewRequest(http.MethodPost, "https://cryptorum.example/api/settings", nil)
	sameReq.Header.Set("Origin", "https://cryptorum.example")
	sameRec := httptest.NewRecorder()
	handler.ServeHTTP(sameRec, sameReq)
	if sameRec.Code != http.StatusNoContent {
		t.Fatalf("same origin status = %d, want 204", sameRec.Code)
	}
}
