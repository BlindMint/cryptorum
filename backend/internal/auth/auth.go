package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Session struct {
	ID        string
	UserID    int64
	Username  string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Store struct {
	db              *sql.DB
	sessionDuration time.Duration
}

func NewStore(db *sql.DB, sessionDuration time.Duration) *Store {
	if sessionDuration <= 0 {
		sessionDuration = 720 * time.Hour
	}
	return &Store{
		db:              db,
		sessionDuration: sessionDuration,
	}
}

func (s *Store) GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashSessionID(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Store) CreateSession(userID int64, username string) (*Session, error) {
	sessionID, err := s.GenerateSessionID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	expiresAt := now.Add(s.sessionDuration)

	session := &Session{
		ID:        sessionID,
		UserID:    userID,
		Username:  username,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	_, err = s.db.Exec(`
		INSERT INTO auth_session (token_hash, user_id, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
	`, hashSessionID(sessionID), userID, now.Unix(), expiresAt.Unix(), now.Unix())
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (s *Store) ValidateSession(sessionID string) (*Session, error) {
	now := time.Now()
	tokenHash := hashSessionID(sessionID)

	var session Session
	var createdAt int64
	var expiresAt int64
	err := s.db.QueryRow(`
		SELECT s.user_id, u.username, s.created_at, s.expires_at
		FROM auth_session s
		JOIN app_user u ON u.id = s.user_id
		WHERE s.token_hash = ?
		  AND s.revoked_at IS NULL
		  AND s.expires_at > ?
	`, tokenHash, now.Unix()).Scan(&session.UserID, &session.Username, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	session.ID = sessionID
	session.CreatedAt = time.Unix(createdAt, 0)
	session.ExpiresAt = time.Unix(expiresAt, 0)

	_, _ = s.db.Exec(`
		UPDATE auth_session
		SET last_seen_at = ?
		WHERE token_hash = ? AND last_seen_at < ?
	`, now.Unix(), tokenHash, now.Add(-1*time.Minute).Unix())

	return &session, nil
}

func (s *Store) DeleteSession(sessionID string) {
	_, _ = s.db.Exec(`
		UPDATE auth_session
		SET revoked_at = ?
		WHERE token_hash = ? AND revoked_at IS NULL
	`, time.Now().Unix(), hashSessionID(sessionID))
}

func (s *Store) CleanupExpired() {
	_, _ = s.db.Exec(`
		DELETE FROM auth_session
		WHERE expires_at <= ? OR revoked_at IS NOT NULL
	`, time.Now().Unix())
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func VerifyPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
