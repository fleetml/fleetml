//go:build integration

package alert

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/dbtest"
)

func TestRepo_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	r := NewRepo(pool)

	var projectID, versionID string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO projects (name, slug) VALUES ('a', $1) RETURNING id::text`, "alerts-"+uuid.NewString()[:8]).Scan(&projectID))
	require.NoError(t, pool.QueryRow(ctx, `WITH m AS (INSERT INTO models (project_id, name, framework, task_type) VALUES ($1, 'detector', 'ONNX', 'DETECTION') RETURNING id)
		INSERT INTO model_versions (model_id, version) SELECT id, '2.0.0' FROM m RETURNING id::text`, projectID).Scan(&versionID))
	p, name, version, err := r.DescribeVersion(ctx, versionID)
	require.NoError(t, err)
	assert.Equal(t, []string{projectID, "detector", "2.0.0"}, []string{p, name, version})
	_, _, _, err = r.DescribeVersion(ctx, uuid.NewString())
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = r.Create(ctx, SinkConfig{ProjectID: uuid.NewString(), Name: "x", Kind: KindWebhook, Config: map[string]string{}})
	assert.ErrorIs(t, err, ErrNotFound, "unknown project")
	on, err := r.Create(ctx, SinkConfig{ProjectID: projectID, Name: "on", Kind: KindSlack, Enabled: true, Config: map[string]string{"webhook_url": "https://h.example/x"}})
	require.NoError(t, err)
	off, err := r.Create(ctx, SinkConfig{ProjectID: projectID, Name: "off", Kind: KindWebhook, Enabled: false, Config: map[string]string{"url": "https://h.example/y"}})
	require.NoError(t, err)
	enabled, err := r.EnabledSinks(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	assert.Equal(t, "https://h.example/x", enabled[0].Config["webhook_url"])

	off.Enabled, off.Name = true, "now on"
	_, err = r.Update(ctx, *off)
	require.NoError(t, err)
	all, err := r.List(ctx, projectID)
	require.NoError(t, err)
	assert.Equal(t, []string{"on", "now on"}, []string{all[0].Name, all[1].Name})

	// The log keeps the newest DeliveryLogSize attempts.
	alertID := uuid.NewString()
	for i := 1; i <= DeliveryLogSize+5; i++ {
		require.NoError(t, r.RecordDelivery(ctx, Delivery{SinkID: on.ID, AlertID: alertID, AlertType: TypeDrift, Attempt: i, Status: StatusError, Error: "boom"}))
	}
	log, err := r.Deliveries(ctx, on.ID, 100)
	require.NoError(t, err)
	require.Len(t, log, DeliveryLogSize)
	assert.Equal(t, DeliveryLogSize+5, log[0].Attempt, "newest first")
	assert.Equal(t, alertID, log[0].AlertID)

	require.NoError(t, r.Delete(ctx, on.ID))
	assert.ErrorIs(t, r.Delete(ctx, on.ID), ErrNotFound)
	_, err = r.Get(ctx, on.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}
