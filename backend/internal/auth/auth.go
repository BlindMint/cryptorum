package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
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
	sessions        map[string]*Session
	secretKey       []byte
	sessionDuration time.Duration
	mu              sync.RWMutex
}

func NewStore(secretKey string, sessionDuration time.Duration) *Store {
	return &Store{
		sessions:        make(map[string]*Session),
		secretKey:       []byte(secretKey),
		sessionDuration: sessionDuration,
	}
}

func (s *Store) GenerateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func (s *Store) CreateSession(userID int64, username string) (*Session, error) {
	sessionID := s.GenerateSessionID()
	now := time.Now()

	session := &Session{
		ID:        sessionID,
		UserID:    userID,
		Username:  username,
		CreatedAt: now,
		ExpiresAt: now.Add(s.sessionDuration),
	}

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	return session, nil
}

func (s *Store) ValidateSession(sessionID string) (*Session, error) {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	if time.Now().After(session.ExpiresAt) {
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		return nil, nil
	}

	return session, nil
}

func (s *Store) DeleteSession(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

func (s *Store) CleanupExpired() {
	s.mu.Lock()
	now := time.Now()
	for id, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
}

func (s *Store) SignSession(sessionID string) string {
	h := hmac.New(sha256.New, s.secretKey)
	h.Write([]byte(sessionID))
	return base64.URLEncoding.EncodeToString(h.Sum(nil))
}

func (s *Store) VerifySignature(sessionID, signature string) bool {
	expected := s.SignSession(sessionID)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func VerifyPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
