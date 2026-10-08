package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MinBootstrapKeyLength is the shortest FLEETML_ADMIN_KEY accepted.
const MinBootstrapKeyLength = 32

// APIKey is an admin key for the REST API. The key itself is never stored.
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// APIKeyStore persists admin API keys.
type APIKeyStore interface {
	Create(ctx context.Context, name, keyHash string) (*APIKey, error)
	// GetActiveByHash returns the non-revoked key with this hash, or ErrNotFound.
	GetActiveByHash(ctx context.Context, keyHash string) (*APIKey, error)
	List(ctx context.Context) ([]APIKey, error)
	// Revoke marks a key revoked unless it is the last active one (ErrLastKey).
	Revoke(ctx context.Context, id uuid.UUID) error
	Touch(ctx context.Context, id uuid.UUID) error
	Count(ctx context.Context) (int, error)
}

// APIKeyService issues and checks admin API keys.
type APIKeyService struct {
	store APIKeyStore
}

// NewAPIKeyService creates an APIKeyService.
func NewAPIKeyService(store APIKeyStore) *APIKeyService {
	return &APIKeyService{store: store}
}

// Authenticate returns the active key matching the presented plaintext key.
func (s *APIKeyService) Authenticate(ctx context.Context, key string) (*APIKey, error) {
	if key == "" {
		return nil, ErrInvalidKey
	}
	k, err := s.store.GetActiveByHash(ctx, HashKey(key))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidKey
		}
		return nil, err
	}
	// last_used_at is informational; a failed update must not deny access.
	_ = s.store.Touch(ctx, k.ID)
	return k, nil
}

// Create issues a new key. The returned plaintext is shown once.
func (s *APIKeyService) Create(ctx context.Context, name string) (string, *APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	key, hash, err := GenerateKey()
	if err != nil {
		return "", nil, err
	}
	k, err := s.store.Create(ctx, name, hash)
	if err != nil {
		return "", nil, err
	}
	return key, k, nil
}

// List returns all keys, including revoked ones.
func (s *APIKeyService) List(ctx context.Context) ([]APIKey, error) {
	return s.store.List(ctx)
}

// Revoke revokes a key. The last active key cannot be revoked, so an operator
// cannot lock themselves out of the API.
func (s *APIKeyService) Revoke(ctx context.Context, id uuid.UUID) error {
	return s.store.Revoke(ctx, id)
}

// Bootstrap makes sure a fresh install has an admin key. It does nothing once
// any key exists. If envKey is set its hash is stored as "bootstrap";
// otherwise a key is generated and returned so the caller can show it once.
func (s *APIKeyService) Bootstrap(ctx context.Context, envKey string) (generated string, err error) {
	n, err := s.store.Count(ctx)
	if err != nil {
		return "", fmt.Errorf("counting api keys: %w", err)
	}
	if n > 0 {
		return "", nil
	}
	hash := ""
	if envKey != "" {
		if len(envKey) < MinBootstrapKeyLength {
			return "", fmt.Errorf("%w: FLEETML_ADMIN_KEY must be at least %d characters", ErrInvalidInput, MinBootstrapKeyLength)
		}
		hash = HashKey(envKey)
	} else {
		generated, hash, err = GenerateKey()
		if err != nil {
			return "", err
		}
	}
	if _, err := s.store.Create(ctx, "bootstrap", hash); err != nil {
		return "", fmt.Errorf("storing bootstrap key: %w", err)
	}
	return generated, nil
}
