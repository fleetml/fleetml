package auth

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memEnrollStore struct {
	mu     sync.Mutex
	keys   map[uuid.UUID]*EnrollmentKey
	hashes map[uuid.UUID]string
}

func newMemEnrollStore() *memEnrollStore {
	return &memEnrollStore{keys: map[uuid.UUID]*EnrollmentKey{}, hashes: map[uuid.UUID]string{}}
}

func (m *memEnrollStore) Create(_ context.Context, projectID uuid.UUID, name, keyHash string) (*EnrollmentKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := &EnrollmentKey{ID: uuid.New(), ProjectID: projectID, Name: name, CreatedAt: time.Now()}
	m.keys[k.ID] = k
	m.hashes[k.ID] = keyHash
	return k, nil
}

func (m *memEnrollStore) GetActiveByHash(_ context.Context, keyHash string) (*EnrollmentKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, h := range m.hashes {
		if h == keyHash && m.keys[id].RevokedAt == nil {
			return m.keys[id], nil
		}
	}
	return nil, fmt.Errorf("lookup: %w", ErrNotFound)
}

func (m *memEnrollStore) ListByProject(_ context.Context, projectID uuid.UUID) ([]EnrollmentKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []EnrollmentKey{}
	for _, k := range m.keys {
		if k.ProjectID == projectID {
			out = append(out, *k)
		}
	}
	return out, nil
}

func (m *memEnrollStore) Revoke(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.keys[id]
	if !ok {
		return ErrNotFound
	}
	now := time.Now()
	k.RevokedAt = &now
	return nil
}

func (m *memEnrollStore) Rotate(ctx context.Context, id uuid.UUID, newKeyHash string) (*EnrollmentKey, error) {
	m.mu.Lock()
	old, ok := m.keys[id]
	if !ok || old.RevokedAt != nil {
		m.mu.Unlock()
		return nil, ErrNotFound
	}
	now := time.Now()
	old.RevokedAt = &now
	m.mu.Unlock()
	return m.Create(ctx, old.ProjectID, old.Name, newKeyHash)
}

func (m *memEnrollStore) Touch(context.Context, uuid.UUID) error { return nil }

func TestEnrollmentService_Lifecycle(t *testing.T) {
	ctx := context.Background()
	svc := NewEnrollmentService(newMemEnrollStore())
	project := uuid.New()

	key, rec, err := svc.Create(ctx, project, "factory line 1")
	require.NoError(t, err)
	assert.Equal(t, project, rec.ProjectID)

	got, err := svc.VerifyEnrollmentKey(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, project, got)

	_, err = svc.VerifyEnrollmentKey(ctx, "unknown")
	assert.ErrorIs(t, err, ErrInvalidKey)
	_, err = svc.VerifyEnrollmentKey(ctx, "")
	assert.ErrorIs(t, err, ErrInvalidKey)

	// Rotate: new key works, old key stops working immediately.
	newKey, newRec, err := svc.Rotate(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, rec.Name, newRec.Name)
	assert.NotEqual(t, key, newKey)
	_, err = svc.VerifyEnrollmentKey(ctx, key)
	assert.ErrorIs(t, err, ErrInvalidKey)
	_, err = svc.VerifyEnrollmentKey(ctx, newKey)
	require.NoError(t, err)

	_, _, err = svc.Rotate(ctx, rec.ID)
	assert.ErrorIs(t, err, ErrNotFound, "a revoked key cannot be rotated")

	require.NoError(t, svc.Revoke(ctx, newRec.ID))
	_, err = svc.VerifyEnrollmentKey(ctx, newKey)
	assert.ErrorIs(t, err, ErrInvalidKey)

	keys, err := svc.List(ctx, project)
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	_, _, err = svc.Create(ctx, project, " ")
	assert.ErrorIs(t, err, ErrInvalidInput)
}
