package agent

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

// fakeServer answers Register, Heartbeat and ReportBatch.
type fakeServer struct {
	fleetmlv1.UnimplementedDeviceServiceServer
	fleetmlv1.UnimplementedTelemetryServiceServer
	mu     sync.Mutex
	events int
}

func (f *fakeServer) Register(context.Context, *fleetmlv1.RegisterRequest) (*fleetmlv1.RegisterResponse, error) {
	return &fleetmlv1.RegisterResponse{DeviceId: "dev-1", AuthToken: "token", TokenExpiresAt: time.Now().Add(24 * time.Hour).Unix()}, nil
}

func (f *fakeServer) Heartbeat(context.Context, *fleetmlv1.HeartbeatRequest) (*fleetmlv1.HeartbeatResponse, error) {
	return &fleetmlv1.HeartbeatResponse{}, nil
}

func (f *fakeServer) ReportBatch(_ context.Context, b *fleetmlv1.TelemetryBatch) (*fleetmlv1.TelemetryAck, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events += len(b.Events)
	return &fleetmlv1.TelemetryAck{AcceptedCount: uint32(len(b.Events))}, nil
}

func startFake(t *testing.T) (*fakeServer, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	f := &fakeServer{}
	gs := grpc.NewServer()
	fleetmlv1.RegisterDeviceServiceServer(gs, f)
	fleetmlv1.RegisterTelemetryServiceServer(gs, f)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	return f, lis.Addr().String()
}

// proxy forwards TCP to a backend. Paused, it holds every byte without
// closing anything, as a network that drops packets silently does; dropped,
// it also closes the connections it had.
type proxy struct {
	backend string
	lis     net.Listener
	mu      sync.Mutex
	paused  bool
	resumed chan struct{}
	conns   []net.Conn
}

func startProxy(t *testing.T, backend string) *proxy {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &proxy{backend: backend, lis: lis, resumed: make(chan struct{})}
	close(p.resumed)
	t.Cleanup(func() { _ = lis.Close(); p.closeAll() })
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			go p.serve(c)
		}
	}()
	return p
}

func (p *proxy) addr() string { return p.lis.Addr().String() }

func (p *proxy) gate() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.resumed
}

func (p *proxy) serve(c net.Conn) {
	<-p.gate() // a new connection waits while the network is down
	b, err := net.Dial("tcp", p.backend)
	if err != nil {
		_ = c.Close()
		return
	}
	p.mu.Lock()
	p.conns = append(p.conns, c, b)
	p.mu.Unlock()
	pipe := func(dst, src net.Conn) {
		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				<-p.gate()
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				_ = dst.Close()
				return
			}
		}
	}
	go pipe(b, c)
	go pipe(c, b)
}

func (p *proxy) pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.paused {
		p.paused, p.resumed = true, make(chan struct{})
	}
}

func (p *proxy) resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.paused {
		p.paused = false
		close(p.resumed)
	}
}

func (p *proxy) closeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
	p.conns = nil
}

// drop closes every connection and holds new ones until resume.
func (p *proxy) drop() {
	p.pause()
	p.closeAll()
}

// syncBuffer is a log destination that tests read while the agent writes.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Reset()
}

func offlineAgent(t *testing.T, server string) (*Agent, *syncBuffer) {
	t.Helper()
	logs := &syncBuffer{}
	dir := t.TempDir()
	cfg := &Config{Server: server, EnrollmentKey: "ek", StateDir: dir, ModelsDir: dir + "/models", HeartbeatInterval: time.Minute}
	a, err := New(cfg, slog.New(slog.NewTextHandler(logs, nil)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })
	a.callTimeout, a.flushTimeout = 500*time.Millisecond, 300*time.Millisecond
	return a, logs
}

// timed runs f and returns how long it took.
func timed(f func()) time.Duration {
	start := time.Now()
	f()
	return time.Since(start)
}

// A server address that accepts connections and never answers: the
// heartbeat fails within the call deadline instead of hanging.
func TestHeartbeatFailsWithinDeadlineOnBlackhole(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = lis.Close() }()
	var mu sync.Mutex
	var held []net.Conn
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range held {
			_ = c.Close()
		}
	}()
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c) // accepted, never read or written
			mu.Unlock()
		}
	}()
	a, logs := offlineAgent(t, lis.Addr().String())
	require.NoError(t, a.store.Update(func(s *State) {
		s.DeviceID, s.Token, s.Signature = "dev-1", "token", a.signature()
		s.TokenIssued, s.TokenExpires = time.Now(), time.Now().Add(24*time.Hour)
	}))
	var ok bool
	took := timed(func() { ok = a.Sync(context.Background()) })
	assert.False(t, ok)
	assert.Less(t, took, 3*time.Second, "bounded by the call deadline")
	assert.Contains(t, logs.String(), "level=WARN msg=\"heartbeat failed;")
}

// A silent drop (bytes held, nothing closed), then a dropped connection: the
// heartbeat fails within the deadline, is logged at WARN once, and the first
// heartbeat after the network is back succeeds and is logged at INFO.
func TestHeartbeatRecoversAfterSilentDrop(t *testing.T) {
	_, backend := startFake(t)
	p := startProxy(t, backend)
	a, logs := offlineAgent(t, p.addr())
	ctx := context.Background()
	require.True(t, a.Sync(ctx), "registers and heartbeats")

	for _, down := range []func(){p.pause, p.drop} {
		down()
		var ok bool
		took := timed(func() { ok = a.Sync(ctx) })
		assert.False(t, ok)
		assert.Less(t, took, 3*time.Second)
		assert.False(t, a.Sync(ctx), "still down")
		assert.Equal(t, 1, strings.Count(logs.String(), "heartbeat failed;"), "WARN once per outage")

		p.resume()
		took = timed(func() { ok = a.Sync(ctx) })
		assert.True(t, ok, "the first heartbeat after the network is back")
		assert.Less(t, took, 3*time.Second)
		assert.Contains(t, logs.String(), "level=INFO msg=\"server reachable again\"")
		logs.Reset()
	}
}

// SIGTERM while the server is unreachable: Run returns within the flush
// timeout, and the events stay queued.
func TestRunExitsWhileOffline(t *testing.T) {
	fake, backend := startFake(t)
	p := startProxy(t, backend)
	a, _ := offlineAgent(t, p.addr())
	require.True(t, a.Sync(context.Background()))
	a.report("v1", fleetmlv1.TelemetryEvent_INFERENCE, 5, 0.9, "", "")
	require.Equal(t, 1, a.queue.Len())
	p.pause()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return while the server was unreachable")
	}
	assert.Equal(t, 1, a.queue.Len(), "the event stays queued for the next start")
	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Zero(t, fake.events)
}

// A heartbeat cut short by the agent's own shutdown is not a server failure:
// no WARN that would read like a new outage.
func TestShutdownCancelledHeartbeatIsNotAWarning(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = lis.Close() }()
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			defer func() { _ = c.Close() }() // accepted, never answered
		}
	}()
	a, logs := offlineAgent(t, lis.Addr().String())
	a.callTimeout = 10 * time.Second
	require.NoError(t, a.store.Update(func(s *State) {
		s.DeviceID, s.Token, s.Signature = "dev-1", "token", a.signature()
		s.TokenIssued, s.TokenExpires = time.Now(), time.Now().Add(24*time.Hour)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	var ok bool
	took := timed(func() { ok = a.Sync(ctx) })
	assert.False(t, ok)
	assert.Less(t, took, 2*time.Second, "the cancel ends the call")
	assert.NotContains(t, logs.String(), "level=WARN", logs.String())
}
