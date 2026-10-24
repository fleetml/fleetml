package rest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPublicMessage(t *testing.T) {
	for in, want := range map[string]string{
		"project.GetByID: not found":                                           "project not found",
		"deployment.GetByID: not found":                                        "deployment not found",
		`project.Create: already exists: slug "tour"`:                          `project already exists: slug "tour"`,
		"enrollmentKey.Rotate: not found":                                      "enrollment key not found",
		"apiKey.Revoke: not found":                                             "API key not found",
		"selecting variant: not found: no compatible variant for device":       "selecting variant: not found: no compatible variant for device",
		"invalid status transition: cannot transition from COMPLETE to PAUSED": "invalid status transition: cannot transition from COMPLETE to PAUSED",
		"invalid input: name is required":                                      "invalid input: name is required",
		"project_id must be a valid UUID":                                      "project_id must be a valid UUID",
		"interval must be one of: 1m, 5m, 15m, 1h, 1d":                         "interval must be one of: 1m, 5m, 15m, 1h, 1d",
		"version.Create: already exists: version \"1.0.0\" for model":          "version already exists: version \"1.0.0\" for model",
	} {
		assert.Equal(t, want, publicMessage(in), in)
	}
}
