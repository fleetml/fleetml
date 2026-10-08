package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memKeyStore struct {
	mu     sync.Mutex
	keys   map[uuid.UUID]*APIKey
	hashes map[uuid.UUID]string
}

func newMemKeyStore() *memKeyStore {
	return &memKeyStore{keys: map[uuid.UUID]*APIKey{}, hashes: map[uuid.UUID]string{}}
}

func (m *memKeyStore) Create(_ context.Context, userID uuid.UUID, name, keyHash string) (*APIKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := &APIKey{ID: uuid.New(), UserID: userID, Name: name, CreatedAt: time.Now()}
	m.keys[k.ID] = k
	m.hashes[k.ID] = keyHash
	return k, nil
}

func (m *memKeyStore) GetActiveByHash(_ context.Context, keyHash string) (*APIKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, h := range m.hashes {
		if h == keyHash && m.keys[id].RevokedAt == nil {
			return m.keys[id], nil
		}
	}
	return nil, fmt.Errorf("apiKey.GetActiveByHash: %w", ErrNotFound)
}

func (m *memKeyStore) List(_ context.Context) ([]APIKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []APIKey{}
	for _, k := range m.keys {
		out = append(out, *k)
	}
	return out, nil
}

func (m *memKeyStore) Revoke(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.keys[id]
	if !ok {
		return ErrNotFound
	}
	if k.RevokedAt != nil {
		return nil
	}
	active := 0
	for _, other := range m.keys {
		if other.RevokedAt == nil && other.ID != id {
			active++
		}
	}
	if active == 0 {
		return ErrLastKey
	}
	now := time.Now()
	k.RevokedAt = &now
	return nil
}

func (m *memKeyStore) Touch(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.keys[id].LastUsedAt = &now
	return nil
}

func (m *memKeyStore) Count(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.keys), nil
}

func TestGenerateKey(t *testing.T) {
	key, hash, err := GenerateKey()
	require.NoError(t, err)
	raw, err := base64.RawURLEncoding.DecodeString(key)
	require.NoError(t, err)
	assert.Len(t, raw, 32)
	assert.Equal(t, HashKey(key), hash)
	assert.Len(t, hash, 64)

	other, _, err := GenerateKey()
	require.NoError(t, err)
	assert.NotEqual(t, key, other)
}

func TestAPIKeyService_CreateAuthenticateRevoke(t *testing.T) {
	ctx := context.Background()
	svc := NewAPIKeyService(newMemKeyStore())

	k1, rec1, err := svc.Create(ctx, BootstrapUserID, "ci")
	require.NoError(t, err)
	_, rec2, err := svc.Create(ctx, BootstrapUserID, "laptop")
	require.NoError(t, err)

	got, err := svc.Authenticate(ctx, k1)
	require.NoError(t, err)
	assert.Equal(t, rec1.ID, got.ID)
	assert.NotNil(t, got.LastUsedAt)

	_, err = svc.Authenticate(ctx, "not-a-key")
	assert.ErrorIs(t, err, ErrInvalidKey)
	_, err = svc.Authenticate(ctx, "")
	assert.ErrorIs(t, err, ErrInvalidKey)

	require.NoError(t, svc.Revoke(ctx, rec1.ID))
	_, err = svc.Authenticate(ctx, k1)
	assert.ErrorIs(t, err, ErrInvalidKey, "revoked key must stop working")

	assert.ErrorIs(t, svc.Revoke(ctx, rec2.ID), ErrLastKey, "the last active key cannot be revoked")

	_, _, err = svc.Create(ctx, BootstrapUserID, "  ")
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestAPIKeyService_Bootstrap(t *testing.T) {
	ctx := context.Background()

	t.Run("env key stored once", func(t *testing.T) {
		store := newMemKeyStore()
		svc := NewAPIKeyService(store)
		envKey := "an-operator-chosen-key-of-at-least-32-chars"
		gen, err := svc.Bootstrap(ctx, envKey)
		require.NoError(t, err)
		assert.Empty(t, gen)
		k, err := svc.Authenticate(ctx, envKey)
		require.NoError(t, err)
		assert.Equal(t, "bootstrap", k.Name)

		// A second start does nothing, even with a different key.
		_, err = svc.Bootstrap(ctx, "another-operator-chosen-key-32-chars-long")
		require.NoError(t, err)
		n, _ := store.Count(ctx)
		assert.Equal(t, 1, n)
	})

	t.Run("generated when env unset", func(t *testing.T) {
		svc := NewAPIKeyService(newMemKeyStore())
		gen, err := svc.Bootstrap(ctx, "")
		require.NoError(t, err)
		require.NotEmpty(t, gen)
		_, err = svc.Authenticate(ctx, gen)
		require.NoError(t, err)
	})

	t.Run("short env key rejected", func(t *testing.T) {
		svc := NewAPIKeyService(newMemKeyStore())
		_, err := svc.Bootstrap(ctx, "short")
		assert.ErrorIs(t, err, ErrInvalidInput)
	})
}
