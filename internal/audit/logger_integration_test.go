//go:build integration

package audit

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/dbtest"
)

func TestLogger_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	l := NewLogger(pool)
	project := uuid.New()
	ok, denied := 201, 403
	require.NoError(t, l.Record(ctx, Entry{Actor: "otto", Action: "POST /deployments", ResourceType: "project", ResourceID: project,
		ProjectID: &project, Status: &ok, Details: map[string]any{"outcome": "allowed"}}))
	require.NoError(t, l.Record(ctx, Entry{Actor: "vera", Action: "POST /deployments", ResourceType: "project", ResourceID: project,
		ProjectID: &project, Status: &denied, IP: "10.0.0.7", Details: map[string]any{"outcome": "denied"}}))
	require.NoError(t, l.Log(ctx, "system", "finalize", "model_variant", uuid.NewString(), nil))

	all, err := l.Query(ctx, Filter{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, all, 3)
	assert.Equal(t, "system", all[0].Actor, "newest first")

	d, err := l.Query(ctx, Filter{ProjectID: &project, Outcome: "denied", Limit: 10})
	require.NoError(t, err)
	require.Len(t, d, 1)
	assert.Equal(t, "vera", d[0].Actor)
	assert.Equal(t, "10.0.0.7", d[0].IP)

	byActor, err := l.Query(ctx, Filter{Actor: "otto", Action: "deployments", From: time.Now().Add(-time.Hour), Limit: 10})
	require.NoError(t, err)
	require.Len(t, byActor, 1)
	none, err := l.Query(ctx, Filter{To: time.Now().Add(-time.Hour), Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, none)
}
