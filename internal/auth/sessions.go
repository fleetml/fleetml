package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidSession means the session is unknown, expired or revoked, or its
// API key was revoked.
var ErrInvalidSession = errors.New("session expired or revoked")

// Session is a dashboard login: an API key exchanged for a cookie. The
// session token itself is never stored.
type Session struct {
	ID         uuid.UUID  `json:"id"`
	APIKeyID   uuid.UUID  `json:"api_key_id"`
	APIKeyName string     `json:"api_key_name"`
	UserAgent  string     `json:"user_agent"`
	IP         string     `json:"ip"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// SessionStore persists sessions.
type SessionStore interface {
	Create(ctx context.Context, apiKeyID uuid.UUID, tokenHash, userAgent, ip string, expiresAt time.Time) (*Session, error)
	// GetActiveByHash returns the unexpired, unrevoked session with this hash
	// whose API key is active, or ErrNotFound.
	GetActiveByHash(ctx context.Context, tokenHash string) (*Session, error)
	ListActive(ctx context.Context) ([]Session, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	Touch(ctx context.Context, id uuid.UUID) error
}

// SessionService logs API keys in and checks session cookies and CSRF tokens.
type SessionService struct {
	store  SessionStore
	keys   *APIKeyService
	secret []byte
	ttl    time.Duration
}

// NewSessionService creates a SessionService; secret derives the CSRF tokens.
func NewSessionService(store SessionStore, keys *APIKeyService, secret string, ttl time.Duration) *SessionService {
	return &SessionService{store: store, keys: keys, secret: []byte(secret), ttl: ttl}
}

// Login exchanges an API key for a new session token (for the cookie).
func (s *SessionService) Login(ctx context.Context, apiKey, userAgent, ip string) (token string, sess *Session, err error) {
	k, err := s.keys.Authenticate(ctx, apiKey)
	if err != nil {
		return "", nil, err
	}
	token, hash, err := GenerateKey()
	if err != nil {
		return "", nil, err
	}
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	sess, err = s.store.Create(ctx, k.ID, hash, userAgent, ip, time.Now().Add(s.ttl))
	if err != nil {
		return "", nil, err
	}
	sess.APIKeyName = k.Name
	return token, sess, nil
}

// Authenticate returns the active session for a cookie token.
func (s *SessionService) Authenticate(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, ErrInvalidSession
	}
	sess, err := s.store.GetActiveByHash(ctx, HashKey(token))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	_ = s.store.Touch(ctx, sess.ID) // informational
	return sess, nil
}

// CSRFToken is the token mutating requests with this session must send. It
// is derived from the session token, so it changes with every login.
func (s *SessionService) CSRFToken(sessionToken string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte("fleetml-csrf:" + sessionToken))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// CheckCSRF compares a presented CSRF token in constant time.
func (s *SessionService) CheckCSRF(sessionToken, presented string) bool {
	return presented != "" && hmac.Equal([]byte(s.CSRFToken(sessionToken)), []byte(presented))
}

// List returns the active sessions, newest first.
func (s *SessionService) List(ctx context.Context) ([]Session, error) {
	return s.store.ListActive(ctx)
}

// Revoke ends a session; its cookie stops working at once.
func (s *SessionService) Revoke(ctx context.Context, id uuid.UUID) error {
	return s.store.Revoke(ctx, id)
}
