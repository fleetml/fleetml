package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EnrollmentKey lets devices register into one project. The key itself is never stored.
type EnrollmentKey struct {
	ID         uuid.UUID  `json:"id"`
	ProjectID  uuid.UUID  `json:"project_id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// EnrollmentKeyStore persists enrollment keys.
type EnrollmentKeyStore interface {
	// Create returns ErrNotFound if the project does not exist.
	Create(ctx context.Context, projectID uuid.UUID, name, keyHash string) (*EnrollmentKey, error)
	// GetActiveByHash returns the non-revoked key with this hash, or ErrNotFound.
	GetActiveByHash(ctx context.Context, keyHash string) (*EnrollmentKey, error)
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]EnrollmentKey, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	// Rotate atomically revokes an active key and creates a replacement with
	// the same project and name. Returns ErrNotFound if id is not active.
	Rotate(ctx context.Context, id uuid.UUID, newKeyHash string) (*EnrollmentKey, error)
	Touch(ctx context.Context, id uuid.UUID) error
}

// EnrollmentService issues and checks per-project enrollment keys.
type EnrollmentService struct {
	store EnrollmentKeyStore
}

// NewEnrollmentService creates an EnrollmentService.
func NewEnrollmentService(store EnrollmentKeyStore) *EnrollmentService {
	return &EnrollmentService{store: store}
}

// Create issues a new enrollment key for a project. The plaintext is shown once.
func (s *EnrollmentService) Create(ctx context.Context, projectID uuid.UUID, name string) (string, *EnrollmentKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	key, hash, err := GenerateKey()
	if err != nil {
		return "", nil, err
	}
	k, err := s.store.Create(ctx, projectID, name, hash)
	if err != nil {
		return "", nil, err
	}
	return key, k, nil
}

// List returns all enrollment keys of a project, including revoked ones.
func (s *EnrollmentService) List(ctx context.Context, projectID uuid.UUID) ([]EnrollmentKey, error) {
	return s.store.ListByProject(ctx, projectID)
}

// Revoke revokes an enrollment key. Devices already enrolled keep working.
func (s *EnrollmentService) Revoke(ctx context.Context, id uuid.UUID) error {
	return s.store.Revoke(ctx, id)
}

// Rotate replaces a key with a new one; the old key stops working immediately.
func (s *EnrollmentService) Rotate(ctx context.Context, id uuid.UUID) (string, *EnrollmentKey, error) {
	key, hash, err := GenerateKey()
	if err != nil {
		return "", nil, err
	}
	k, err := s.store.Rotate(ctx, id, hash)
	if err != nil {
		return "", nil, err
	}
	return key, k, nil
}

// VerifyEnrollmentKey returns the project an active enrollment key belongs to,
// or ErrInvalidKey.
func (s *EnrollmentService) VerifyEnrollmentKey(ctx context.Context, key string) (uuid.UUID, error) {
	if key == "" {
		return uuid.Nil, ErrInvalidKey
	}
	k, err := s.store.GetActiveByHash(ctx, HashKey(key))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return uuid.Nil, ErrInvalidKey
		}
		return uuid.Nil, err
	}
	_ = s.store.Touch(ctx, k.ID)
	return k.ProjectID, nil
}
