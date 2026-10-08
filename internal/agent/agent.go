package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/google/uuid"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	"github.com/fleetml/fleetml/internal/artifact"
	"github.com/fleetml/fleetml/internal/features"
)

// AgentVersion is the agent's version, reported at registration.
const AgentVersion = "0.2.0"

// modelFile is the name of the model inside each version directory.
const modelFile = "model.bin"

// Agent keeps one machine's models in line with what the server assigns.
type Agent struct {
	cfg     *Config
	store   *Store
	queue   *Queue
	log     *slog.Logger
	http    *http.Client
	conn    *grpc.ClientConn
	devices fleetmlv1.DeviceServiceClient
	tel     fleetmlv1.TelemetryServiceClient
	now     func() time.Time

	mu         sync.Mutex
	interval   time.Duration
	batchSize  int
	flushEvery time.Duration
	policy     fleetmlv1.DownloadPolicy
	arms       map[string]string // version ID -> experiment arm ID
	// reconfigured wakes the telemetry loop when the server changes its interval.
	reconfigured chan struct{}
	// featureBins are the bin edges of each model's features (model ID -> feature -> edges).
	featureBins map[string]map[string][]float64
}

// New opens the agent's state and connects to the server (lazily).
func New(cfg *Config, log *slog.Logger) (*Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	store, err := OpenStore(filepath.Join(cfg.StateDir, "state.json"))
	if err != nil {
		return nil, err
	}
	if err := store.Update(func(s *State) {
		if s.Salt == "" {
			s.Salt = NewSalt()
		}
	}); err != nil {
		return nil, fmt.Errorf("saving state: %w", err)
	}
	queue, err := OpenQueue(filepath.Join(cfg.StateDir, "telemetry.queue"))
	if err != nil {
		return nil, fmt.Errorf("opening telemetry queue: %w", err)
	}
	creds := insecure.NewCredentials()
	if cfg.TLS {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	conn, err := grpc.NewClient(cfg.Server, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", cfg.Server, err)
	}
	return &Agent{
		cfg: cfg, store: store, queue: queue, log: log, http: &http.Client{Timeout: 30 * time.Minute},
		conn: conn, devices: fleetmlv1.NewDeviceServiceClient(conn), tel: fleetmlv1.NewTelemetryServiceClient(conn),
		now: time.Now, interval: cfg.HeartbeatInterval, batchSize: 100, flushEvery: 30 * time.Second, arms: map[string]string{}, reconfigured: make(chan struct{}, 1),
	}, nil
}

// Close releases the connection.
func (a *Agent) Close() error { return a.conn.Close() }

// Run heartbeats and sends telemetry until ctx ends. Models activated in an
// earlier run stay in place (their `current` symlinks) while the server is
// unreachable.
func (a *Agent) Run(ctx context.Context) error {
	for _, m := range a.ActiveModels() {
		a.log.Info("active model (from the last run)", "model", m.Name, "version", m.Active.Version, "path", a.currentLink(m.Name))
	}
	if a.cfg.TelemetryListen != "" {
		srv, err := a.serveLocal(a.cfg.TelemetryListen)
		if err != nil {
			return err
		}
		defer func() { _ = srv.Close() }()
	}
	go a.telemetryLoop(ctx)
	for {
		a.Sync(ctx)
		a.mu.Lock()
		wait := a.interval
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			a.flush(context.Background())
			return nil
		case <-time.After(wait):
		}
	}
}

// Sync registers if needed, sends one heartbeat and applies the response.
// It returns true if the server was reached.
func (a *Agent) Sync(ctx context.Context) bool {
	if a.needsRegistration() {
		if err := a.register(ctx); err != nil {
			a.log.Warn("registration failed", "error", err)
			return false
		}
	}
	if err := a.heartbeat(ctx); err != nil {
		if status.Code(err) == codes.Unauthenticated {
			a.log.Warn("token rejected, registering again")
			_ = a.store.Update(func(s *State) { s.Token = "" })
		} else {
			a.log.Warn("heartbeat failed", "error", err)
		}
		return false
	}
	return true
}

func (a *Agent) signature() string {
	keys := make([]string, 0, len(a.cfg.Labels))
	for k := range a.cfg.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + a.cfg.Labels[k]
	}
	return AgentVersion + "|" + strings.Join(parts, ",")
}

func (a *Agent) needsRegistration() bool {
	need := false
	a.store.Read(func(s *State) { need = s.Token == "" || s.Signature != a.signature() })
	return need
}

func (a *Agent) register(ctx context.Context) error {
	var salt, token string
	a.store.Read(func(s *State) { salt, token = s.Salt, s.Token })
	req := &fleetmlv1.RegisterRequest{
		Fingerprint: Fingerprint(MachineID(), salt), Os: "LINUX", OsVersion: osVersion(), Chipset: chipset(),
		RamMb: MemoryMB(), AgentVersion: "fleetml-agent/" + AgentVersion, Labels: a.cfg.Labels, EnrollmentKey: a.cfg.EnrollmentKey,
	}
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}
	resp, err := a.devices.Register(ctx, req)
	if err != nil {
		return err
	}
	sig := a.signature()
	if err := a.store.Update(func(s *State) {
		s.DeviceID, s.Token, s.Signature = resp.DeviceId, resp.AuthToken, sig
		s.TokenExpires, s.TokenIssued = time.Unix(resp.TokenExpiresAt, 0), a.now()
		s.SigningKeys = signingKeys(resp.SigningKeys)
	}); err != nil {
		return err
	}
	a.applyConfig(resp.Config)
	a.log.Info("registered", "device_id", resp.DeviceId)
	return nil
}

func (a *Agent) authCtx(ctx context.Context) context.Context {
	var token string
	a.store.Read(func(s *State) { token = s.Token })
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

func (a *Agent) heartbeat(ctx context.Context) error {
	var deviceID string
	var issued, expires time.Time
	a.store.Read(func(s *State) { deviceID, issued, expires = s.DeviceID, s.TokenIssued, s.TokenExpires })
	if refreshDue(a.now(), issued, expires) {
		if resp, err := a.devices.RefreshToken(a.authCtx(ctx), &fleetmlv1.RefreshTokenRequest{}); err == nil {
			_ = a.store.Update(func(s *State) {
				s.Token, s.TokenExpires, s.TokenIssued = resp.AuthToken, time.Unix(resp.TokenExpiresAt, 0), a.now()
			})
			a.log.Info("device token refreshed")
		} else if status.Code(err) == codes.Unauthenticated {
			return err
		}
	}
	req := &fleetmlv1.HeartbeatRequest{DeviceId: deviceID}
	for id, m := range a.models() {
		req.LoadedModels = append(req.LoadedModels, &fleetmlv1.ModelStatus{
			ModelId: id, Version: m.Active.Version, VariantId: m.Active.VariantID, Status: "LOADED",
		})
	}
	resp, err := a.devices.Heartbeat(a.authCtx(ctx), req)
	if err != nil {
		return err
	}
	// Keys come before assignments: an assignment may be signed by a new key.
	_ = a.store.Update(func(s *State) { s.SigningKeys = signingKeys(resp.SigningKeys) })
	if resp.Config != nil {
		a.applyConfig(resp.Config)
	} else if resp.NextHeartbeatSeconds > 0 {
		a.setInterval(time.Duration(resp.NextHeartbeatSeconds) * time.Second)
	}
	arms := map[string]string{}
	for _, e := range resp.Experiments {
		arms[e.ModelVersionId] = e.ArmId
	}
	a.mu.Lock()
	a.arms = arms
	a.mu.Unlock()
	a.apply(ctx, resp.Assignments)
	return nil
}

func (a *Agent) setInterval(d time.Duration) {
	if d >= time.Second {
		a.mu.Lock()
		a.interval = d
		a.mu.Unlock()
	}
}

// applyConfig takes the server's DeviceConfig. A Linux agent runs all the time,
// so it uses the foreground heartbeat interval.
func (a *Agent) applyConfig(c *fleetmlv1.DeviceConfig) {
	if c == nil {
		return
	}
	a.setInterval(time.Duration(c.HeartbeatIntervalSeconds) * time.Second)
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.TelemetryBatchSize > 0 {
		a.batchSize = int(c.TelemetryBatchSize)
	}
	if d := time.Duration(c.TelemetryFlushIntervalSeconds) * time.Second; d > 0 && d != a.flushEvery {
		a.flushEvery = d
		select {
		case a.reconfigured <- struct{}{}:
		default:
		}
	}
	if c.DownloadPolicy != fleetmlv1.DownloadPolicy_DOWNLOAD_POLICY_UNSPECIFIED {
		a.policy = c.DownloadPolicy
	}
	a.featureBins = map[string]map[string][]float64{}
	for _, b := range c.FeatureBins {
		if a.featureBins[b.ModelId] == nil {
			a.featureBins[b.ModelId] = map[string][]float64{}
		}
		a.featureBins[b.ModelId][b.Feature] = b.Edges
	}
}

// histograms bins an inference's feature values with the model's configured
// edges; features the server did not configure are left out.
func (a *Agent) histograms(modelID string, values map[string][]float64) []*fleetmlv1.FeatureHistogram {
	a.mu.Lock()
	bins := a.featureBins[modelID]
	a.mu.Unlock()
	var out []*fleetmlv1.FeatureHistogram
	for name, v := range values {
		if edges, ok := bins[name]; ok {
			out = append(out, &fleetmlv1.FeatureHistogram{Name: name, Counts: features.Bin(edges, v)})
		}
	}
	return out
}

// models returns the models with an active version, by model ID.
func (a *Agent) models() map[string]*ModelState {
	out := map[string]*ModelState{}
	a.store.Read(func(s *State) {
		for id, m := range s.Models {
			if m.Active != nil {
				cp := *m
				out[id] = &cp
			}
		}
	})
	return out
}

// ActiveModels lists the active models, sorted by name.
func (a *Agent) ActiveModels() []ModelState {
	var out []ModelState
	for _, m := range a.models() {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (a *Agent) apply(ctx context.Context, assignments []*fleetmlv1.ModelAssignment) {
	// A replacement arrives as UPDATE (new) + UNLOAD (old): the old version
	// stays until the new one is active, so such UNLOADs are ignored.
	replacing := map[string]bool{}
	for _, as := range assignments {
		if as.Action == fleetmlv1.ModelAssignment_LOAD || as.Action == fleetmlv1.ModelAssignment_UPDATE {
			replacing[as.ModelId] = true
		}
	}
	for _, as := range assignments {
		switch as.Action {
		case fleetmlv1.ModelAssignment_LOAD, fleetmlv1.ModelAssignment_UPDATE:
			a.install(ctx, as)
		case fleetmlv1.ModelAssignment_UNLOAD:
			if !replacing[as.ModelId] {
				a.unload(as)
			}
		}
	}
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeName(s string) string {
	s = unsafeName.ReplaceAllString(s, "_")
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	return s
}

func (a *Agent) currentLink(modelName string) string {
	return filepath.Join(a.cfg.ModelsDir, safeName(modelName), "current")
}

func (a *Agent) install(ctx context.Context, as *fleetmlv1.ModelAssignment) {
	var active *Version
	var failedAt time.Time
	a.store.Read(func(s *State) {
		if m := s.Models[as.ModelId]; m != nil {
			active = m.Active
		}
		failedAt = s.Failed[as.VariantId]
	})
	if active != nil && active.VariantID == as.VariantId {
		return
	}
	if !failedAt.IsZero() && a.now().Sub(failedAt) < failedCooldown {
		return
	}
	// The signature covers the sha256 the download is checked against, so a
	// model that fails it is not even downloaded.
	var keys []artifact.Key
	a.store.Read(func(s *State) { keys = s.SigningKeys })
	if err := artifact.Verify(keys, as.SigningKeyId, as.Signature, as.ModelId, as.Version, as.BlobSha256, a.now()); err != nil {
		a.log.Warn("model signature rejected; keeping the current version", "model", as.ModelName, "version", as.Version, "error", err)
		_ = a.store.Update(func(s *State) { s.Failed[as.VariantId] = a.now() })
		a.report(as.VersionId, fleetmlv1.TelemetryEvent_SIGNATURE_INVALID, 0, 0, "", err.Error())
		return
	}
	dir := filepath.Join(a.cfg.ModelsDir, safeName(as.ModelName), safeName(as.Version)+"-"+safeName(shortID(as.VariantId)))
	file := filepath.Join(dir, modelFile)
	if sum, err := fileSHA256(file); err != nil || sum != as.BlobSha256 {
		a.mu.Lock()
		policy := a.policy
		a.mu.Unlock()
		if !MayDownload(policy, as.Urgent, a.cfg.Metered, a.cfg.OnBattery) {
			a.log.Info("download waits for the download policy", "model", as.ModelName, "version", as.Version)
			return
		}
		part := filepath.Join(a.cfg.StateDir, "downloads", safeName(as.VariantId)+".part")
		if err := download(ctx, a.http, as.DownloadUrl, part, as.BlobSha256); err != nil {
			a.log.Warn("download failed", "model", as.ModelName, "version", as.Version, "error", err)
			return // retried on the next heartbeat
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			a.log.Error("creating model directory failed", "error", err)
			return
		}
		if err := os.Rename(part, file); err != nil {
			a.log.Error("moving model into place failed", "error", err)
			return
		}
	}

	started := a.now()
	env := map[string]string{"FLEETML_MODEL_NAME": as.ModelName, "FLEETML_MODEL_VERSION": as.Version, "FLEETML_MODEL_FILE": file}
	if a.cfg.Validator != "" {
		if err := runCommand(ctx, a.cfg.Validator, file, env); err != nil {
			a.log.Warn("model failed validation; keeping the current version", "model", as.ModelName, "version", as.Version, "error", err)
			_ = a.store.Update(func(s *State) { s.Failed[as.VariantId] = a.now() })
			if active == nil || active.Dir != dir {
				_ = os.RemoveAll(dir)
			}
			a.report(as.VersionId, fleetmlv1.TelemetryEvent_MODEL_LOAD_FAILED, 0, 0, "", err.Error())
			return
		}
	}
	if err := swapSymlink(a.currentLink(as.ModelName), filepath.Base(dir)); err != nil {
		a.log.Error("switching the current symlink failed", "error", err)
		return
	}
	next := &Version{VersionID: as.VersionId, Version: as.Version, VariantID: as.VariantId, SHA256: as.BlobSha256, Dir: dir}
	var keep []string
	_ = a.store.Update(func(s *State) {
		m := s.Models[as.ModelId]
		if m == nil {
			m = &ModelState{}
			s.Models[as.ModelId] = m
		}
		m.Name = as.ModelName
		if m.Active != nil && m.Active.VariantID != as.VariantId {
			m.LastKnownGood = m.Active
		}
		m.Active = next
		delete(s.Failed, as.VariantId)
		keep = []string{m.Active.Dir}
		if m.LastKnownGood != nil {
			keep = append(keep, m.LastKnownGood.Dir)
		}
	})
	pruneVersions(filepath.Dir(dir), keep)
	a.report(as.VersionId, fleetmlv1.TelemetryEvent_MODEL_LOAD, uint32(a.now().Sub(started).Milliseconds()), 0, "", "")
	a.log.Info("model active", "model", as.ModelName, "version", as.Version, "path", a.currentLink(as.ModelName))
	if a.cfg.PostActivate != "" {
		env["FLEETML_MODEL_PATH"] = a.currentLink(as.ModelName)
		if err := runCommand(ctx, a.cfg.PostActivate, file, env); err != nil {
			a.log.Warn("post_activate command failed", "error", err)
		}
	}
}

func (a *Agent) unload(as *fleetmlv1.ModelAssignment) {
	var name string
	var drop bool
	_ = a.store.Update(func(s *State) {
		m := s.Models[as.ModelId]
		if m == nil || m.Active == nil || m.Active.VariantID != as.VariantId {
			return
		}
		name, drop = m.Name, true
		delete(s.Models, as.ModelId)
	})
	if drop {
		_ = os.RemoveAll(filepath.Join(a.cfg.ModelsDir, safeName(name)))
		a.log.Info("model unloaded", "model", name)
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// swapSymlink points link at target atomically (rename over the old link).
func swapSymlink(link, target string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	tmp := link + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, link)
}

// pruneVersions deletes version directories of a model other than keep.
func pruneVersions(modelDir string, keep []string) {
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(modelDir, e.Name())
		if !e.IsDir() || e.Type()&os.ModeSymlink != 0 {
			continue
		}
		kept := false
		for _, k := range keep {
			if k == p {
				kept = true
			}
		}
		if !kept {
			_ = os.RemoveAll(p)
		}
	}
}

// report queues a telemetry event; experiment arms are tagged automatically.
func (a *Agent) report(versionID string, typ fleetmlv1.TelemetryEvent_EventType, latencyMs uint32, confidence float32, label, errMsg string,
	hist ...*fleetmlv1.FeatureHistogram) {
	a.mu.Lock()
	arm := a.arms[versionID]
	a.mu.Unlock()
	err := a.queue.Add(&fleetmlv1.TelemetryEvent{
		EventId:        uuid.NewString(), // stored once, even if the batch is sent again
		ModelVersionId: versionID, ExperimentArmId: arm, Type: typ, LatencyMs: latencyMs,
		ConfidenceScore: confidence, PredictionLabel: label, ErrorMessage: errMsg, TimestampUnixMs: a.now().UnixMilli(),
		Features: hist,
	})
	if err != nil {
		a.log.Warn("queueing telemetry failed", "error", err)
	}
}

func (a *Agent) telemetryLoop(ctx context.Context) {
	failures := 0
	for {
		a.mu.Lock()
		wait := a.flushEvery
		a.mu.Unlock()
		if failures > 0 {
			wait = backoff(failures)
		}
		select {
		case <-ctx.Done():
			return
		case <-a.reconfigured:
			continue // a new flush interval applies now, not after the old one
		case <-time.After(wait):
		}
		if a.flush(ctx) {
			failures = 0
		} else {
			failures++
		}
	}
}

func backoff(failures int) time.Duration {
	d := 5 * time.Second << (failures - 1)
	if d > 5*time.Minute || d <= 0 {
		return 5 * time.Minute
	}
	return d
}

// flush sends queued events in batches; false if a send failed (they stay queued).
func (a *Agent) flush(ctx context.Context) bool {
	var deviceID string
	a.store.Read(func(s *State) { deviceID = s.DeviceID })
	if deviceID == "" {
		return true // not registered yet: keep everything queued
	}
	for {
		a.mu.Lock()
		n := a.batchSize
		a.mu.Unlock()
		batch := a.queue.Peek(n)
		if len(batch) == 0 {
			return true
		}
		if _, err := a.tel.ReportBatch(a.authCtx(ctx), &fleetmlv1.TelemetryBatch{DeviceId: deviceID, Events: batch}); err != nil {
			a.log.Warn("sending telemetry failed", "error", err)
			return false
		}
		if err := a.queue.Remove(len(batch)); err != nil {
			a.log.Warn("trimming the telemetry queue failed", "error", err)
		}
	}
}

func signingKeys(in []*fleetmlv1.SigningKey) []artifact.Key {
	out := make([]artifact.Key, 0, len(in))
	for _, k := range in {
		out = append(out, artifact.Key{KeyID: k.KeyId, PublicKey: k.PublicKey, NotAfter: k.NotAfter})
	}
	return out
}
