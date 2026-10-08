package health

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecker(t *testing.T) {
	c := NewChecker(nil)
	c.Add("db", func(context.Context) error { return nil })
	statuses, ok := c.Check(context.Background())
	assert.True(t, ok)
	assert.Equal(t, map[string]string{"db": "connected"}, statuses)

	c.Add("s3", func(context.Context) error { return errors.New("down") })
	statuses, ok = c.Check(context.Background())
	assert.False(t, ok)
	assert.Equal(t, map[string]string{"db": "connected", "s3": "disconnected"}, statuses)
}
