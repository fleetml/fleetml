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
}

// serveLocal starts the local telemetry endpoint: POST /v1/events with one
// event or an array queues inference events for the active version of each
// model; GET /v1/models lists the active models.
func (a *Agent) serveLocal(addr string) (*http.Server, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events", a.handleEvents)
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		type model struct{ Name, Version, VersionID, Path string }
		var out []model
		for _, m := range a.ActiveModels() {
			out = append(out, model{m.Name, m.Active.Version, m.Active.VersionID, a.currentLink(m.Name)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
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
	byName := map[string]string{}
	for _, m := range a.ActiveModels() {
		byName[m.Name] = m.Active.VersionID
	}
	accepted := 0
	for _, e := range events {
		versionID, ok := byName[e.Model]
		if !ok {
			continue
		}
		a.report(versionID, fleetmlv1.TelemetryEvent_INFERENCE, e.LatencyMs, e.Confidence, e.Label, e.Error)
		accepted++
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]int{"accepted": accepted, "rejected": len(events) - accepted})
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
