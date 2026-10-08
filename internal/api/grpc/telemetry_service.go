package grpc

import (
	"context"
	"log/slog"
	"time"

	"github.com/fleetml/fleetml/internal/metrics"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	"github.com/fleetml/fleetml/internal/features"
	"github.com/fleetml/fleetml/internal/telemetry"
)

// TelemetryServiceServer implements fleetmlv1.TelemetryServiceServer.
type TelemetryServiceServer struct {
	fleetmlv1.UnimplementedTelemetryServiceServer
	service *telemetry.Service
	logger  *slog.Logger
}

// NewTelemetryServiceServer creates a new TelemetryServiceServer.
func NewTelemetryServiceServer(svc *telemetry.Service, logger *slog.Logger) *TelemetryServiceServer {
	return &TelemetryServiceServer{service: svc, logger: logger}
}

// maxTelemetryBatch bounds one ReportBatch call.
const maxTelemetryBatch = 1000

// ReportBatch handles batch telemetry from devices.
func (s *TelemetryServiceServer) ReportBatch(ctx context.Context, req *fleetmlv1.TelemetryBatch) (*fleetmlv1.TelemetryAck, error) {
	// Extract device_id from JWT context
	deviceID, ok := ctx.Value(deviceIDKey).(uuid.UUID)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing device authentication")
	}

	if n := len(req.GetEvents()); n > maxTelemetryBatch {
		return nil, status.Errorf(codes.InvalidArgument, "batch has %d events; the maximum is %d", n, maxTelemetryBatch)
	}

	events := make([]telemetry.Event, 0, len(req.GetEvents()))
	for _, pe := range req.GetEvents() {
		eventType := pe.GetType().String()
		if eventType == "EVENT_TYPE_UNSPECIFIED" {
			eventType = "INFERENCE"
		}

		var modelVersionID uuid.UUID
		if mvID := pe.GetModelVersionId(); mvID != "" {
			parsed, err := uuid.Parse(mvID)
			if err == nil {
				modelVersionID = parsed
			}
		}

		var armID *uuid.UUID
		if a := pe.GetExperimentArmId(); a != "" {
			if parsed, err := uuid.Parse(a); err == nil {
				armID = &parsed
			}
		}

		recordedAt := time.Now()
		if ts := pe.GetTimestampUnixMs(); ts > 0 {
			recordedAt = time.UnixMilli(ts)
		}

		eventID, _ := uuid.Parse(pe.GetEventId()) // the service assigns one if it is missing or invalid
		events = append(events, telemetry.Event{
			EventID:         eventID,
			DeviceID:        deviceID,
			ModelVersionID:  modelVersionID,
			ExperimentArmID: armID,
			EventType:       eventType,
			LatencyMs:       int(pe.GetLatencyMs()),
			Confidence:      float64(pe.GetConfidenceScore()),
			PredictionLabel: pe.GetPredictionLabel(),
			MemoryUsageMB:   int(pe.GetMemoryUsageMb()),
			BatteryPercent:  float64(pe.GetBatteryPercent()),
			ErrorMessage:    pe.GetErrorMessage(),
			RecordedAt:      recordedAt,
			Features:        featureCounts(pe.GetFeatures()),
		})
	}

	accepted, rejected, err := s.service.IngestEvents(ctx, deviceID, events)
	if err != nil {
		s.logger.Error("telemetry ingestion failed", "device_id", deviceID, "error", err)
		return nil, status.Error(codes.Internal, "ingestion failed")
	}

	metrics.TelemetryAccepted.Add(float64(accepted))
	metrics.TelemetryRejected.Add(float64(rejected))
	return &fleetmlv1.TelemetryAck{
		AcceptedCount: uint32(accepted),
		RejectedCount: uint32(rejected),
	}, nil
}

// featureCounts keeps well-formed histograms (valid name, at most
// features.MaxEdges+1 bins); drift ignores those that do not match the
// model's configured bins.
func featureCounts(in []*fleetmlv1.FeatureHistogram) map[string][]int32 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]int32, len(in))
	for _, h := range in {
		c := h.GetCounts()
		if features.Validate(h.GetName(), []float64{0}) != nil || len(c) == 0 || len(c) > features.MaxEdges+1 {
			continue
		}
		counts := make([]int32, len(c))
		for i, v := range c {
			counts[i] = int32(min(v, 1<<30))
		}
		out[h.GetName()] = counts
	}
	return out
}
