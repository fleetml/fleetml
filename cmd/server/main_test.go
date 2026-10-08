package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWaitFor(t *testing.T) {
	calls := 0
	err := waitFor(context.Background(), "thing", 5*time.Second, func() error {
		calls++
		if calls < 2 {
			return errors.New("not yet")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, calls, "retried until it succeeded")

	start := time.Now()
	err = waitFor(context.Background(), "thing", 0, func() error { return errors.New("down") })
	assert.EqualError(t, err, "down", "the last error is returned after the timeout")
	assert.Less(t, time.Since(start), time.Second)
}
