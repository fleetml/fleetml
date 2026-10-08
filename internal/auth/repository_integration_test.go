//go:build integration

package auth

import (
	"context"
	"testing"
	"time"

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

	key2, rec2, err := svc.Create(ctx, BootstrapUserID, "ci")
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

func TestSessionRepo_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	keys := NewAPIKeyService(NewAPIKeyRepo(pool))
	_, err := keys.Bootstrap(ctx, "")
	require.NoError(t, err)
	key, rec, err := keys.Create(ctx, BootstrapUserID, "dashboard")
	require.NoError(t, err)
	svc := NewSessionService(NewSessionRepo(pool), keys, "integration-secret", time.Hour)

	_, _, err = svc.Login(ctx, "wrong", "ua", "127.0.0.1")
	assert.ErrorIs(t, err, ErrInvalidKey)
	token, sess, err := svc.Login(ctx, key, "Mozilla/5.0", "127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, "dashboard", sess.APIKeyName)
	assert.WithinDuration(t, time.Now().Add(time.Hour), sess.ExpiresAt, time.Minute)

	got, err := svc.Authenticate(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, sess.ID, got.ID)
	assert.Equal(t, rec.ID, got.APIKeyID)
	_, err = svc.Authenticate(ctx, key)
	assert.ErrorIs(t, err, ErrInvalidSession, "the API key is not a session token")

	token2, _, err := svc.Login(ctx, key, "curl", "10.0.0.1")
	require.NoError(t, err)
	list, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)

	require.NoError(t, svc.Revoke(ctx, sess.ID))
	require.NoError(t, svc.Revoke(ctx, sess.ID), "revoking twice is a no-op")
	assert.ErrorIs(t, svc.Revoke(ctx, uuid.New()), ErrNotFound)
	_, err = svc.Authenticate(ctx, token)
	assert.ErrorIs(t, err, ErrInvalidSession)

	// Revoking the API key ends its remaining session.
	require.NoError(t, keys.Revoke(ctx, rec.ID))
	_, err = svc.Authenticate(ctx, token2)
	assert.ErrorIs(t, err, ErrInvalidSession)
	list, err = svc.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)

	// Expired sessions do not authenticate.
	boot, err := keys.List(ctx)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO sessions (api_key_id, token_hash, expires_at) VALUES ($1, $2, NOW() - INTERVAL '1 second')`, boot[0].ID, HashKey("expired-token"))
	require.NoError(t, err)
	_, err = svc.Authenticate(ctx, "expired-token")
	assert.ErrorIs(t, err, ErrInvalidSession)
}

func TestUserRepo_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	users := NewUserService(NewUserRepo(pool))
	keys := NewAPIKeyService(NewAPIKeyRepo(pool))
	f := dbtest.NewVersion(t, pool)

	all, err := users.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1, "the migration creates the bootstrap user")
	assert.Equal(t, BootstrapUserID, all[0].ID)
	assert.True(t, all[0].Admin)

	vera, err := users.Create(ctx, "vera", false)
	require.NoError(t, err)
	_, err = users.Create(ctx, "vera", false)
	assert.ErrorIs(t, err, ErrInvalidInput, "names are unique")
	_, err = users.Create(ctx, "  ", false)
	assert.ErrorIs(t, err, ErrInvalidInput)

	_, rec, err := keys.Create(ctx, vera.ID, "laptop")
	require.NoError(t, err)
	assert.Equal(t, vera.ID, rec.UserID)
	_, _, err = keys.Create(ctx, uuid.New(), "ghost")
	assert.ErrorIs(t, err, ErrNotFound, "a key needs an existing user")

	require.NoError(t, users.SetRole(ctx, vera.ID, f.ProjectID, RoleViewer))
	require.NoError(t, users.SetRole(ctx, vera.ID, f.ProjectID, RoleOperator), "setting again replaces the role")
	assert.ErrorIs(t, users.SetRole(ctx, vera.ID, f.ProjectID, "owner"), ErrInvalidInput)
	assert.ErrorIs(t, users.SetRole(ctx, vera.ID, uuid.New(), RoleViewer), ErrNotFound)
	members, err := users.Members(ctx, f.ProjectID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, Member{UserID: vera.ID, UserName: "vera", Role: RoleOperator}, members[0])

	p, err := users.PrincipalForKey(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, "vera", p.User.Name)
	assert.Equal(t, "laptop", p.Key)
	assert.Equal(t, map[string]string{f.ProjectID.String(): RoleOperator}, p.Roles)

	require.NoError(t, users.RemoveRole(ctx, vera.ID, f.ProjectID))
	assert.ErrorIs(t, users.RemoveRole(ctx, vera.ID, f.ProjectID), ErrNotFound)
}
