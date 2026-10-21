package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

// InferenceEvent is what a local inference process posts to /v1/events.
type InferenceEvent struct {
	// Model is the model name, as in models_dir/<model>/current.
	Model      string  `json:"model"`
	LatencyMs  uint32  `json:"latency_ms"`
	Confidence float32 `json:"confidence"`
	Label      string  `json:"label"`
	// Error marks the inference as failed (counts against the health gate).
	Error string `json:"error"`
	// Features are the input values by feature name, for feature-level
	// drift; only features configured for the model are reported.
	Features map[string][]float64 `json:"features,omitempty"`
}

// serveLocal starts the local telemetry endpoint: POST /v1/events with one
// event or an array queues inference events for the active version of each
// model; GET /v1/models lists the active models.
func (a *Agent) serveLocal(addr string) (*http.Server, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events", a.handleEvents)
	mux.HandleFunc("GET /v1/models", a.handleModels)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("telemetry_listen %s: %w", addr, err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Error("local telemetry endpoint stopped", "error", err)
		}
	}()
	a.log.Info("local telemetry endpoint listening", "addr", ln.Addr().String())
	return srv, nil
}

// LocalModel is one entry of GET /v1/models; the keys match /v1/events.
type LocalModel struct {
	// Model is the model name, as posted to /v1/events.
	Model     string `json:"model"`
	Version   string `json:"version"`
	VersionID string `json:"version_id"`
	// Path is the model's `current` link: read Path/model.bin.
	Path string `json:"path"`
}

func (a *Agent) handleModels(w http.ResponseWriter, _ *http.Request) {
	out := []LocalModel{}
	for _, m := range a.ActiveModels() {
		out = append(out, LocalModel{Model: m.Name, Version: m.Active.Version, VersionID: m.Active.VersionID, Path: a.currentLink(m.Name)})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (a *Agent) handleEvents(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, 1<<20)
	var events []InferenceEvent
	dec := json.NewDecoder(body)
	raw := json.RawMessage{}
	if err := dec.Decode(&raw); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		if err := json.Unmarshal(raw, &events); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
	} else {
		var e InferenceEvent
		if err := json.Unmarshal(raw, &e); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		events = []InferenceEvent{e}
	}
	type active struct{ modelID, versionID string }
	byName := map[string]active{}
	for id, m := range a.models() {
		byName[m.Name] = active{id, m.Active.VersionID}
	}
	resp := EventsResponse{}
	for i, e := range events {
		m, ok := byName[e.Model]
		if !ok {
			resp.RejectedReasons = append(resp.RejectedReasons, fmt.Sprintf("event %d: unknown model %q (%s)", i, e.Model, activeNames(byName)))
			continue
		}
		a.report(m.versionID, fleetmlv1.TelemetryEvent_INFERENCE, e.LatencyMs, e.Confidence, e.Label, e.Error, a.histograms(m.modelID, e.Features)...)
		resp.Accepted++
	}
	resp.Rejected = len(resp.RejectedReasons)
	if resp.Rejected > 0 {
		a.log.Info("local telemetry events rejected", "rejected", resp.Rejected, "first", resp.RejectedReasons[0])
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(resp)
}

// EventsResponse is the answer to POST /v1/events.
type EventsResponse struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
	// RejectedReasons has one entry per rejected event, e.g.
	// `event 0: unknown model "foo" (active models: iris)`.
	RejectedReasons []string `json:"rejected_reasons,omitempty"`
}

func activeNames[T any](byName map[string]T) string {
	if len(byName) == 0 {
		return "no model is active yet"
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	return "active models: " + strings.Join(names, ", ")
}

func osVersion() string {
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
				return strings.Trim(v, `"`)
			}
		}
	}
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		return runtime.GOOS + " " + strings.TrimSpace(string(out))
	}
	return runtime.GOOS
}

func chipset() string { return runtime.GOOS + "/" + runtime.GOARCH }
