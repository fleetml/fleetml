package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fleetml/fleetml/internal/version"
)

// startupListener holds the REST port while the server waits for its
// dependencies: /livez answers 200 at once, so a startup or liveness probe
// does not restart a server that is only waiting, and /readyz answers 503
// with what it waits for. The REST server takes the port over when start-up
// is done.
type startupListener struct {
	waiting atomic.Value // string
	srv     *http.Server
	done    chan struct{}
	once    sync.Once
}

func listenDuringStartup(addr string) (*startupListener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("REST port: %w", err)
	}
	l := &startupListener{done: make(chan struct{})}
	l.waiting.Store("")
	l.srv = &http.Server{Handler: http.HandlerFunc(l.serve), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		defer close(l.done)
		if err := l.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("start-up listener stopped", "error", err)
		}
	}()
	return l, nil
}

func (l *startupListener) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	waiting, _ := l.waiting.Load().(string)
	var body any
	switch r.URL.Path {
	case "/livez":
		body = map[string]string{"status": "ok", "version": version.String()}
	case "/readyz", "/healthz":
		w.WriteHeader(http.StatusServiceUnavailable)
		body = map[string]string{"status": "starting", "waiting_for": waiting, "version": version.String()}
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
		body = map[string]any{"error": map[string]string{"code": "UNAVAILABLE", "message": "the server is starting: waiting for " + waiting}}
	}
	_ = json.NewEncoder(w).Encode(body)
}

// wait records what start-up waits for, for /readyz.
func (l *startupListener) wait(what string) { l.waiting.Store(what) }

// stop frees the port for the REST server; safe to call more than once.
func (l *startupListener) stop() {
	l.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = l.srv.Shutdown(ctx)
		<-l.done
	})
}
