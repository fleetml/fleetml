package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/db/migrations"
)

func TestPgx5URL(t *testing.T) {
	assert.Equal(t, "pgx5://u:p@h:15432/db?sslmode=disable", pgx5URL("postgres://u:p@h:15432/db?sslmode=disable"))
	assert.Equal(t, "pgx5://u:p@h/db", pgx5URL("postgresql://u:p@h/db"))
	assert.Equal(t, "pgx5://already", pgx5URL("pgx5://already"))
}

func TestEmbeddedMigrationsPresent(t *testing.T) {
	entries, err := migrations.FS.ReadDir(".")
	require.NoError(t, err)
	var ups int
	for _, e := range entries {
		if len(e.Name()) > 7 && e.Name()[len(e.Name())-7:] == ".up.sql" {
			ups++
		}
	}
	assert.GreaterOrEqual(t, ups, 5)
}
