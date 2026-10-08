// Package scope carries, in a request's context, the projects the caller may
// see, so list queries that span projects return only those.
package scope

import (
	"context"

	"github.com/google/uuid"
)

type key struct{}

// WithProjects limits ctx to the given projects (an empty list: none).
func WithProjects(ctx context.Context, projects []uuid.UUID) context.Context {
	if projects == nil {
		projects = []uuid.UUID{}
	}
	return context.WithValue(ctx, key{}, projects)
}

// Projects is the query argument for "project_id = ANY($n)": nil (SQL NULL)
// when ctx is not limited, else the visible projects. Use it as
// ($n::uuid[] IS NULL OR project_id = ANY($n)).
func Projects(ctx context.Context) []uuid.UUID {
	p, _ := ctx.Value(key{}).([]uuid.UUID)
	return p
}
