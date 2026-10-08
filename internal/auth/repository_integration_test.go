//go:build integration

package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/dbtest"
)

func TestAPIKeyRepo_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	svc := NewAPIKeyService(NewAPIKeyRepo(pool))

	gen, err := svc.Bootstrap(ctx, "")
	require.NoError(t, err)
	require.NotEmpty(t, gen)
	again, err := svc.Bootstrap(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, again, "bootstrap runs once")

	boot, err := svc.Authenticate(ctx, gen)
	require.NoError(t, err)
	assert.Equal(t, "bootstrap", boot.Name)
	_, err = svc.Authenticate(ctx, "wrong")
	assert.ErrorIs(t, err, ErrInvalidKey)

	key2, rec2, err := svc.Create(ctx, "ci")
	require.NoError(t, err)
	keys, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	require.NoError(t, svc.Revoke(ctx, rec2.ID))
	require.NoError(t, svc.Revoke(ctx, rec2.ID), "revoking twice is a no-op")
	_, err = svc.Authenticate(ctx, key2)
	assert.ErrorIs(t, err, ErrInvalidKey)
	assert.ErrorIs(t, svc.Revoke(ctx, boot.ID), ErrLastKey, "the last active key stays")
	assert.ErrorIs(t, svc.Revoke(ctx, uuid.New()), ErrNotFound)
}

func TestEnrollmentKeyRepo_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	fx := dbtest.NewVersion(t, pool)
	svc := NewEnrollmentService(NewEnrollmentKeyRepo(pool))

	_, _, err := svc.Create(ctx, uuid.New(), "x")
	assert.ErrorIs(t, err, ErrNotFound, "unknown project")

	key, rec, err := svc.Create(ctx, fx.ProjectID, "line-1")
	require.NoError(t, err)
	p, err := svc.VerifyEnrollmentKey(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, fx.ProjectID, p)

	newKey, newRec, err := svc.Rotate(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, "line-1", newRec.Name)
	_, err = svc.VerifyEnrollmentKey(ctx, key)
	assert.ErrorIs(t, err, ErrInvalidKey, "rotated key stops working")
	_, err = svc.VerifyEnrollmentKey(ctx, newKey)
	require.NoError(t, err)
	_, _, err = svc.Rotate(ctx, rec.ID)
	assert.ErrorIs(t, err, ErrNotFound, "a revoked key cannot be rotated")

	require.NoError(t, svc.Revoke(ctx, newRec.ID))
	_, err = svc.VerifyEnrollmentKey(ctx, newKey)
	assert.ErrorIs(t, err, ErrInvalidKey)
	assert.ErrorIs(t, svc.Revoke(ctx, uuid.New()), ErrNotFound)

	list, err := svc.List(ctx, fx.ProjectID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}
