package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

// simConfig shapes the simulated fleet.
type simConfig struct {
	grpcAddr      string
	enrollmentKey string
	devices       int
	labels        map[string]string
	interval      time.Duration
	events        int
	seed          string
	latency       map[string]float64 // version -> mean latency (ms)
	errorRate     map[string]float64 // version -> share of events that are errors
	history       time.Duration
}

func newSimulateCmd() *cobra.Command {
	var c simConfig
	var labels, latency, errorRate []string
	var duration time.Duration
	cmd := &cobra.Command{
		Use:   "simulate",
		Short: "Run a fleet of simulated devices against a server",
		Long: `Runs simulated devices that register with an enrollment key, heartbeat,
download and sha256-verify the models they are assigned, and report inference
telemetry. Per-version latency and error rates let you exercise health gates,
experiments and drift detection without real hardware. Devices are derived from
--seed, so a second run with the same seed continues as the same devices.`,
		Example: `  FLEETML_ENROLLMENT_KEY=$KEY fleetml simulate --devices 20 --label app=demo \
    --error-rate 2.0.0=0.3 --latency 2.1.0=14 --duration 1m`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error
			if c.labels, err = parsePairs(labels); err != nil {
				return err
			}
			if c.latency, err = parseFloats(latency); err != nil {
				return err
			}
			if c.errorRate, err = parseFloats(errorRate); err != nil {
				return err
			}
			if c.enrollmentKey == "" {
				c.enrollmentKey = os.Getenv("FLEETML_ENROLLMENT_KEY")
			}
			if c.enrollmentKey == "" {
				return fmt.Errorf("--enrollment-key or FLEETML_ENROLLMENT_KEY is required")
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			if duration > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, duration)
				defer cancel()
			}
			return runSimulation(ctx, c, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&c.grpcAddr, "grpc", "localhost:19090", "Device gRPC address")
	f.StringVar(&c.enrollmentKey, "enrollment-key", "", "Project enrollment key (required; or FLEETML_ENROLLMENT_KEY, which keeps it out of the process list)")
	f.IntVar(&c.devices, "devices", 10, "Number of devices")
	f.StringArrayVar(&labels, "label", nil, "Device label key=value (repeatable)")
	f.DurationVar(&c.interval, "interval", time.Second, "Heartbeat and telemetry interval")
	f.IntVar(&c.events, "events", 5, "Inference events per device, model and interval")
	f.StringVar(&c.seed, "seed", "fleetml-sim", "Seed for device identities")
	f.StringArrayVar(&latency, "latency", nil, "Mean inference latency for a version, version=ms (default 20)")
	f.StringArrayVar(&errorRate, "error-rate", nil, "Share of failing inferences for a version, version=0..1 (repeatable)")
	f.DurationVar(&c.history, "history", 0, "Also report this much past telemetry (ending a day ago) for each newly loaded model, as a drift baseline")
	f.DurationVar(&duration, "duration", 0, "Stop after this long (default: until interrupted)")
	return cmd
}

func parsePairs(items []string) (map[string]string, error) {
	out := map[string]string{}
	for _, it := range items {
		k, v, ok := strings.Cut(it, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("expected key=value, got %q", it)
		}
		out[k] = v
	}
	return out, nil
}

func parseFloats(items []string) (map[string]float64, error) {
	pairs, err := parsePairs(items)
	if err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for k, v := range pairs {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a number", k, v)
		}
		out[k] = f
	}
	return out, nil
}

// simDevice is one simulated device: what it runs and its experiment arms.
type simDevice struct {
	n         int    // its number in the fleet, for log lines
	fp        string // its fingerprint, from the seed
	id, token string
	loaded    map[string]*fleetmlv1.ModelAssignment // model ID -> running assignment
	arms      map[string]string                     // version ID -> arm ID
	seen      map[string]bool                       // versions that already got history
	rng       *rand.Rand
}

func runSimulation(ctx context.Context, c simConfig, out io.Writer) error {
	conn, err := grpc.NewClient(c.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	devSvc, telSvc := fleetmlv1.NewDeviceServiceClient(conn), fleetmlv1.NewTelemetryServiceClient(conn)

	devs := make([]*simDevice, c.devices)
	for i := range devs {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d", c.seed, i)))
		devs[i] = &simDevice{n: i, fp: hex.EncodeToString(sum[:]), loaded: map[string]*fleetmlv1.ModelAssignment{},
			arms: map[string]string{}, seen: map[string]bool{}, rng: rand.New(rand.NewSource(int64(sum[0])<<8 | int64(sum[1])))}
		if err := devs[i].register(ctx, c, devSvc); err != nil {
			return fmt.Errorf("registering device %d: %w", i, err)
		}
	}
	_, _ = fmt.Fprintf(out, "simulator: %d devices registered\n", len(devs))

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	lastStatus := ""
	for {
		var wg sync.WaitGroup
		for _, d := range devs {
			wg.Add(1)
			go func(d *simDevice) {
				defer wg.Done()
				d.tick(ctx, c, devSvc, telSvc, out)
			}(d)
		}
		wg.Wait()
		if s := fleetStatus(devs); s != lastStatus {
			_, _ = fmt.Fprintf(out, "simulator: %s\n", s)
			lastStatus = s
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// fleetStatus summarises which versions the fleet runs, e.g. "iris 1.0.0 on 15, iris 2.0.0 on 5".
func fleetStatus(devs []*simDevice) string {
	counts := map[string]int{}
	for _, d := range devs {
		for _, a := range d.loaded {
			counts[a.ModelName+" "+a.Version]++
		}
	}
	if len(counts) == 0 {
		return "no models assigned yet"
	}
	parts := make([]string, 0, len(counts))
	for k, n := range counts {
		parts = append(parts, fmt.Sprintf("%s on %d", k, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// register registers the device (again) with the enrollment key and keeps its new token.
func (d *simDevice) register(ctx context.Context, c simConfig, devSvc fleetmlv1.DeviceServiceClient) error {
	resp, err := devSvc.Register(ctx, &fleetmlv1.RegisterRequest{
		Fingerprint: d.fp, Os: "ANDROID", OsVersion: "14", Chipset: "simulated",
		RamMb: 6144, HasNpu: true, HasGpu: true, AgentVersion: "simulator", Labels: c.labels, EnrollmentKey: c.enrollmentKey,
	})
	if err != nil {
		return err
	}
	d.id, d.token = resp.DeviceId, resp.AuthToken
	return nil
}

// simOut serialises the devices' log lines, which come from their goroutines.
var simOut sync.Mutex

func (d *simDevice) tick(ctx context.Context, c simConfig, devSvc fleetmlv1.DeviceServiceClient, telSvc fleetmlv1.TelemetryServiceClient, out io.Writer) {
	auth := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+d.token)
	req := &fleetmlv1.HeartbeatRequest{DeviceId: d.id}
	for _, a := range d.loaded {
		req.LoadedModels = append(req.LoadedModels, &fleetmlv1.ModelStatus{ModelId: a.ModelId, Version: a.Version, VariantId: a.VariantId, Status: "LOADED"})
	}
	resp, err := devSvc.Heartbeat(auth, req)
	if status.Code(err) == codes.Unauthenticated {
		// The server refused the token (its JWT secret rotated, or the device was
		// revoked): register again with the enrollment key, like the SDKs do.
		rerr := d.register(ctx, c, devSvc)
		simOut.Lock()
		if rerr != nil {
			_, _ = fmt.Fprintf(out, "simulator: device %d: token refused, registering again failed: %v\n", d.n, rerr)
		} else {
			_, _ = fmt.Fprintf(out, "simulator: device %d: token refused, registered again\n", d.n)
		}
		simOut.Unlock()
		return // the next tick sends a heartbeat with the new token
	}
	if err != nil {
		return // the next tick retries, like the SDKs
	}
	d.arms = map[string]string{}
	for _, e := range resp.Experiments {
		d.arms[e.ModelVersionId] = e.ArmId
	}
	// A replacement arrives as UPDATE (new) + UNLOAD (old): keep the model until the new one is in place.
	replacing := map[string]bool{}
	for _, a := range resp.Assignments {
		if a.Action == fleetmlv1.ModelAssignment_LOAD || a.Action == fleetmlv1.ModelAssignment_UPDATE {
			replacing[a.ModelId] = true
		}
	}
	var events []*fleetmlv1.TelemetryEvent
	for _, a := range resp.Assignments {
		switch a.Action {
		case fleetmlv1.ModelAssignment_LOAD, fleetmlv1.ModelAssignment_UPDATE:
			if err := download(ctx, a.DownloadUrl, a.BlobSha256); err != nil {
				_, _ = fmt.Fprintf(out, "simulator: %s %s download failed: %v\n", a.ModelName, a.Version, err)
				continue
			}
			d.loaded[a.ModelId] = a
			events = append(events, &fleetmlv1.TelemetryEvent{ModelVersionId: a.VersionId, Type: fleetmlv1.TelemetryEvent_MODEL_LOAD, LatencyMs: 30})
			if c.history > 0 && !d.seen[a.VersionId] {
				d.seen[a.VersionId] = true
				d.sendHistory(auth, c, telSvc, a)
			}
		case fleetmlv1.ModelAssignment_UNLOAD:
			if cur, ok := d.loaded[a.ModelId]; ok && cur.VariantId == a.VariantId && !replacing[a.ModelId] {
				delete(d.loaded, a.ModelId)
			}
		}
	}
	now := time.Now()
	for _, a := range d.loaded {
		for i := 0; i < c.events; i++ {
			events = append(events, d.event(c, a, now))
		}
	}
	if len(events) > 0 {
		_, _ = telSvc.ReportBatch(auth, &fleetmlv1.TelemetryBatch{DeviceId: d.id, Events: events})
	}
}

// event is one inference by version a at time t, shaped by the version's profile.
func (d *simDevice) event(c simConfig, a *fleetmlv1.ModelAssignment, t time.Time) *fleetmlv1.TelemetryEvent {
	mean := 20.0
	if m, ok := c.latency[a.Version]; ok {
		mean = m
	}
	lat := d.rng.NormFloat64()*mean*0.15 + mean
	if lat < 1 {
		lat = 1
	}
	e := &fleetmlv1.TelemetryEvent{
		ModelVersionId: a.VersionId, ExperimentArmId: d.arms[a.VersionId], Type: fleetmlv1.TelemetryEvent_INFERENCE,
		LatencyMs: uint32(lat), ConfidenceScore: float32(0.85 + d.rng.Float64()*0.1), PredictionLabel: "class-a",
		TimestampUnixMs: t.UnixMilli(),
	}
	if d.rng.Float64() < c.errorRate[a.Version] {
		e.Type, e.ErrorMessage = fleetmlv1.TelemetryEvent_ERROR, "simulated inference error"
	}
	return e
}

// sendHistory reports past inferences spread over [now-1d-history, now-1d].
func (d *simDevice) sendHistory(ctx context.Context, c simConfig, telSvc fleetmlv1.TelemetryServiceClient, a *fleetmlv1.ModelAssignment) {
	end := time.Now().Add(-24 * time.Hour)
	const n = 200
	batch := make([]*fleetmlv1.TelemetryEvent, 0, n)
	for i := 0; i < n; i++ {
		t := end.Add(-time.Duration(d.rng.Int63n(int64(c.history))))
		e := d.event(simConfig{latency: map[string]float64{}, errorRate: map[string]float64{}}, a, t)
		e.ExperimentArmId = ""
		batch = append(batch, e)
	}
	_, _ = telSvc.ReportBatch(ctx, &fleetmlv1.TelemetryBatch{DeviceId: d.id, Events: batch})
}

func download(ctx context.Context, url, wantSHA string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	h := sha256.New()
	if _, err := io.Copy(h, resp.Body); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
		return fmt.Errorf("sha256 mismatch: got %s", got[:12])
	}
	return nil
}
