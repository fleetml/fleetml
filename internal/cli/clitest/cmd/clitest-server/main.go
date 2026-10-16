//go:build clitest

// Command clitest-server serves clitest's fake of the REST API on a port, so
// a released fleetml binary can run the quickstart's CLI calls where the real
// server cannot run (the Windows run workflow on GitHub's Windows runners). It
// is a test fixture, built only with the clitest build tag and never released:
//
//	go build -tags clitest -o clitest-server ./internal/cli/clitest/cmd/clitest-server
//	clitest-server -addr 127.0.0.1:18081 -key <admin key>
//
// The key can also come from CLITEST_API_KEY. It answers /healthz and /readyz
// without a key, and /api/v1 with the key as "Authorization: Bearer".
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/fleetml/fleetml/internal/cli/clitest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18081", "address to listen on")
	key := flag.String("key", os.Getenv("CLITEST_API_KEY"), "admin API key the fake accepts (default $CLITEST_API_KEY)")
	flag.Parse()
	if *key == "" {
		fmt.Fprintln(os.Stderr, "clitest-server: set -key or CLITEST_API_KEY")
		os.Exit(2)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "clitest-server:", err)
		os.Exit(1)
	}
	fmt.Printf("clitest-server: fake REST API on http://%s\n", ln.Addr())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err = serve(ctx, ln, *key)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "clitest-server:", err)
		os.Exit(1)
	}
}

// serve answers on ln until ctx is done.
func serve(ctx context.Context, ln net.Listener, key string) error {
	srv := &http.Server{Handler: clitest.NewHandler(key), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
