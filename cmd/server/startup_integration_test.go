//go:build integration

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/config"
	"github.com/fleetml/fleetml/internal/dbtest"
)

// lateProxy forwards addr to target, but only starts listening after delay:
// a database that comes up late, as on a cold Kubernetes node.
func lateProxy(t *testing.T, addr, target string, delay time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			t.Errorf("late proxy: %v", err)
			return
		}
		go func() { <-ctx.Done(); _ = ln.Close() }()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				up, err := net.Dial("tcp", target)
				if err != nil {
					_ = c.Close()
					return
				}
				go func() { _, _ = io.Copy(up, c); _ = up.Close() }()
				_, _ = io.Copy(c, up)
				_ = c.Close()
			}()
		}
	}()
}

// The server answers /livez while it waits for a database that is not there
// yet, and moves on to the next dependency once the database arrives.
func TestStartup_LateDatabase_Integration(t *testing.T) {
	adminURL := os.Getenv(dbtest.EnvURL)
	if adminURL == "" {
		t.Skipf("%s not set", dbtest.EnvURL)
	}
	ctx := context.Background()
	name := "fleetml_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	admin, err := pgx.Connect(ctx, adminURL)
	require.NoError(t, err)
	_, err = admin.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})

	u, err := url.Parse(adminURL)
	require.NoError(t, err)
	pgTarget := u.Host
	proxyAddr := freeAddr(t)
	u.Host, u.Path = proxyAddr, "/"+name
	lateProxy(t, proxyAddr, pgTarget, 3*time.Second)

	rest, grpc := freeAddr(t), freeAddr(t)
	port := func(a string) string { _, p, _ := net.SplitHostPort(a); return p }
	for k, v := range map[string]string{
		"DATABASE_URL": u.String(), "MIGRATE_ON_START": "true",
		"S3_ENDPOINT": freeAddr(t), "S3_ACCESS_KEY": "a", "S3_SECRET_KEY": "b", // nothing listens: start-up stays at s3
		"JWT_SECRET": strings.Repeat("j", 32), "SIGNING_MASTER_KEY": strings.Repeat("s", 32),
		"REST_PORT": port(rest), "GRPC_PORT": port(grpc), "DEPENDENCY_TIMEOUT": "60s",
	} {
		t.Setenv(k, v)
	}
	cfg, err := config.Load()
	require.NoError(t, err)

	sctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- serve(sctx, cancel, cfg) }()

	base := "http://127.0.0.1:" + strconv.Itoa(cfg.RESTPort)
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/livez")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 2*time.Second, 50*time.Millisecond, "/livez answers before the database is there")
	assert.Less(t, time.Since(start), 3*time.Second)
	code, body := getJSON(t, base+"/readyz")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "database migrations", body["waiting_for"])

	require.Eventually(t, func() bool {
		_, body := getJSON(t, base+"/readyz")
		return body["waiting_for"] == "s3"
	}, 20*time.Second, 200*time.Millisecond, "migrations and the database done once it arrives")
	code, _ = getJSON(t, base+"/livez")
	assert.Equal(t, http.StatusOK, code)

	cancel()
	select {
	case err := <-done:
		assert.ErrorContains(t, err, "preparing s3 bucket")
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after cancel")
	}
}
