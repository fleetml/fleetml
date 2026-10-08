//go:build integration

package device

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/dbtest"
)

func TestDeviceRepo_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	repo := NewDeviceRepo(pool)
	classes := NewDeviceClassRepo(pool)
	fx := dbtest.NewVersion(t, pool)

	hash := "h1"
	d := &Device{ProjectID: &fx.ProjectID, Fingerprint: GenerateFingerprint("ANDROID", "chip", 4096), OS: "ANDROID",
		RAMMB: 4096, Labels: map[string]string{"region": "eu", "tier": "gold"}, AuthTokenHash: &hash}
	require.NoError(t, repo.Create(ctx, d))
	assert.ErrorIs(t, repo.Create(ctx, &Device{Fingerprint: d.Fingerprint, OS: "ANDROID"}), ErrConflict)

	got, err := repo.GetByID(ctx, d.ID)
	require.NoError(t, err)
	assert.Equal(t, fx.ProjectID, *got.ProjectID)
	assert.Equal(t, "eu", got.Labels["region"])
	got, err = repo.GetByFingerprint(ctx, d.Fingerprint)
	require.NoError(t, err)
	assert.Equal(t, d.ID, got.ID)
	_, err = repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = repo.GetByFingerprint(ctx, "missing")
	assert.ErrorIs(t, err, ErrNotFound)

	other := &Device{Fingerprint: GenerateFingerprint("ANDROID", "other", 2048), OS: "ANDROID", Labels: map[string]string{"region": "us"}}
	require.NoError(t, repo.Create(ctx, other))
	all, total, err := repo.List(ctx, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, all, 2)
	eu, total, err := repo.ListByLabels(ctx, map[string]string{"region": "eu"}, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, d.ID, eu[0].ID)

	require.NoError(t, repo.RecordHeartbeat(ctx, d.ID, []string{"variant-a"}))
	got, _ = repo.GetByID(ctx, d.ID)
	assert.NotNil(t, got.LastSeenAt)
	assert.ErrorIs(t, repo.RecordHeartbeat(ctx, uuid.New(), nil), ErrNotFound)

	require.NoError(t, repo.UpdateLabels(ctx, d.ID, map[string]string{"region": "apac"}))
	got.OSVersion, got.RAMMB = "15", 8192
	require.NoError(t, repo.Update(ctx, got))
	got, _ = repo.GetByID(ctx, d.ID)
	assert.Equal(t, "15", got.OSVersion)
	assert.Equal(t, 8192, got.RAMMB)

	require.NoError(t, repo.SetAuthTokenHash(ctx, d.ID, nil))
	got, _ = repo.GetByID(ctx, d.ID)
	assert.Nil(t, got.AuthTokenHash)
	assert.ErrorIs(t, repo.SetAuthTokenHash(ctx, uuid.New(), nil), ErrNotFound)

	require.NoError(t, repo.Delete(ctx, other.ID))
	assert.ErrorIs(t, repo.Delete(ctx, other.ID), ErrNotFound)

	dc := &DeviceClass{Name: "android-midrange", OS: "ANDROID", MinRAMMB: 4096}
	require.NoError(t, classes.Upsert(ctx, dc))
	again := &DeviceClass{Name: "android-midrange", OS: "ANDROID", MinRAMMB: 6144}
	require.NoError(t, classes.Upsert(ctx, again))
	assert.Equal(t, dc.ID, again.ID, "upsert keeps the class ID")
	byName, err := classes.GetByName(ctx, "android-midrange")
	require.NoError(t, err)
	assert.Equal(t, 6144, byName.MinRAMMB)
	_, err = classes.GetByID(ctx, dc.ID)
	require.NoError(t, err)
	require.NoError(t, classes.Create(ctx, &DeviceClass{Name: "linux-edge", OS: "LINUX"}))
	assert.ErrorIs(t, classes.Create(ctx, &DeviceClass{Name: "linux-edge", OS: "LINUX"}), ErrConflict)
	list, err := classes.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	_, err = classes.GetByName(ctx, "nope")
	assert.ErrorIs(t, err, ErrNotFound)
}

// The registration path with real repositories: enrolled, re-registered,
// revoked.
func TestService_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	fx := dbtest.NewVersion(t, pool)
	svc := NewService(NewDeviceRepo(pool), NewDeviceClassRepo(pool), "integration-secret", testLogger())
	svc.SetEnrollmentVerifier(fakeEnrollment{testEnrollKey: fx.ProjectID})

	first, err := svc.RegisterDevice(ctx, &RegisterRequest{EnrollmentKey: testEnrollKey, Fingerprint: validFingerprint(), OS: "android", RAMMB: 4096})
	require.NoError(t, err)
	second, err := svc.RegisterDevice(ctx, &RegisterRequest{CurrentToken: first.AuthToken, Fingerprint: validFingerprint(), OS: "ANDROID", RAMMB: 4096})
	require.NoError(t, err)
	assert.Equal(t, first.DeviceID, second.DeviceID)
	_, _, err = svc.ValidateToken(ctx, first.AuthToken)
	assert.ErrorIs(t, err, ErrUnauthenticated)

	id := uuid.MustParse(second.DeviceID)
	resp, err := svc.Heartbeat(ctx, &HeartbeatRequest{DeviceID: id, LoadedModels: []ModelStatus{{VariantID: fx.VariantID.String()}}})
	require.NoError(t, err)
	assert.Empty(t, resp.Assignments)

	devices, _, err := svc.ListDevicesByLabels(ctx, map[string]string{}, 10, 0)
	require.NoError(t, err)
	assert.Len(t, devices, 1)
	classes, err := svc.ListDeviceClasses(ctx)
	require.NoError(t, err)
	require.Len(t, classes, 1)
	_, err = svc.GetDeviceClass(ctx, classes[0].ID)
	require.NoError(t, err)

	require.NoError(t, svc.RevokeDeviceToken(ctx, id))
	_, _, err = svc.ValidateToken(ctx, second.AuthToken)
	assert.ErrorIs(t, err, ErrUnauthenticated)
}
