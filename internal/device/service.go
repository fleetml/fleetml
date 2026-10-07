package device

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/registry"
)

const (
	tokenExpiry          = 90 * 24 * time.Hour // 90 days
	defaultHeartbeatSecs = 60
	defaultBatchSize     = 100
	defaultMaxCacheMB    = 512
	downloadURLExpiry    = 1 * time.Hour
)

// RegisterRequest is the service-layer request for device registration.
type RegisterRequest struct {
	Fingerprint  string
	OS           string
	OSVersion    string
	Chipset      string
	RAMMB        int
	HasNPU       bool
	HasGPU       bool
	NPUTflops    float64
	AgentVersion string
	Labels       map[string]string
}

// RegisterResponse is the service-layer response for device registration.
type RegisterResponse struct {
	DeviceID              string
	AuthToken             string
	HeartbeatIntervalSecs uint32
	TelemetryBatchSize    uint32
	MaxCacheSizeMB        uint32
}

// HeartbeatRequest is the service-layer request for heartbeat.
type HeartbeatRequest struct {
	DeviceID     uuid.UUID
	LoadedModels []ModelStatus
}

// ModelStatus reports a model currently loaded on the device.
type ModelStatus struct {
	ModelID   string
	Version   string
	VariantID string
	Status    string
}

// HeartbeatResponse is the service-layer response for heartbeat.
type HeartbeatResponse struct {
	Assignments          []ModelAssignment
	Experiments          []ExperimentAssignment
	NextHeartbeatSeconds uint32
}

// ModelAssignment tells the device to load/update/unload a model.
type ModelAssignment struct {
	ModelID       string
	ModelName     string
	Version       string
	VariantID     string
	DownloadURL   string
	BlobSizeBytes uint64
	BlobSHA256    string
	Action        string // LOAD, UPDATE, UNLOAD
}

// ExperimentAssignment is a placeholder for future experiment support.
type ExperimentAssignment struct {
	ExperimentID   string
	ArmName        string
	ModelVersionID string
}

// AssignmentComputer computes model assignments for a device.
// Implemented by the intelligence orchestrator.
type AssignmentComputer interface {
	ComputeAssignments(ctx context.Context, dev *Device, currentlyLoaded []ModelStatus) ([]ModelAssignment, error)
}

// ExperimentAssigner computes experiment-based assignments for a device.
type ExperimentAssigner interface {
	ComputeExperimentAssignments(ctx context.Context, dev *Device, currentlyLoaded []ModelStatus) ([]ModelAssignment, error)
}

// Service orchestrates business logic for device management.
type Service struct {
	devices        DeviceRepository
	classes        DeviceClassRepository
	deployments    DeploymentQuerier
	variants       VariantQuerier
	storage        registry.StorageClient
	assignmentComp AssignmentComputer
	experimentComp ExperimentAssigner
	tokenSecret    string
	logger         *slog.Logger
}

// NewService creates a new device service.
func NewService(
	devices DeviceRepository,
	classes DeviceClassRepository,
	deployments DeploymentQuerier,
	variants VariantQuerier,
	storage registry.StorageClient,
	tokenSecret string,
	logger *slog.Logger,
) *Service {
	return &Service{
		devices:     devices,
		classes:     classes,
		deployments: deployments,
		variants:    variants,
		storage:     storage,
		tokenSecret: tokenSecret,
		logger:      logger,
	}
}

// SetAssignmentComputer sets the assignment computer (intelligence orchestrator).
func (s *Service) SetAssignmentComputer(ac AssignmentComputer) {
	s.assignmentComp = ac
}

// SetExperimentAssigner sets the experiment assigner.
func (s *Service) SetExperimentAssigner(ea ExperimentAssigner) {
	s.experimentComp = ea
}

// RegisterDevice handles first-time device registration or re-registration.
func (s *Service) RegisterDevice(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	if err := ValidateFingerprint(req.Fingerprint); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}

	os := strings.ToUpper(strings.TrimSpace(req.OS))
	validOS := map[string]bool{"ANDROID": true, "IOS": true, "LINUX": true, "WINDOWS": true}
	if !validOS[os] {
		return nil, fmt.Errorf("%w: os must be one of ANDROID, IOS, LINUX, WINDOWS", ErrInvalidInput)
	}

	if req.Labels == nil {
		req.Labels = map[string]string{}
	}

	// Check for existing device (re-registration)
	existing, err := s.devices.GetByFingerprint(ctx, req.Fingerprint)
	if err == nil {
		// Re-registration: update device info
		existing.OS = os
		existing.OSVersion = req.OSVersion
		existing.RAMMB = req.RAMMB
		existing.Chipset = req.Chipset
		existing.HasNPU = req.HasNPU
		existing.HasGPU = req.HasGPU
		existing.NPUTflops = req.NPUTflops
		existing.AgentVersion = req.AgentVersion
		existing.Labels = req.Labels

		// Re-classify
		className := ClassifyDevice(existing)
		dc := s.buildDeviceClass(existing, className)
		if err := s.classes.Upsert(ctx, dc); err != nil {
			return nil, fmt.Errorf("upserting device class: %w", err)
		}
		existing.DeviceClassID = &dc.ID

		// Generate new token
		token, tokenHash, err := s.generateToken(existing.ID, existing.Fingerprint)
		if err != nil {
			return nil, fmt.Errorf("generating token: %w", err)
		}
		existing.AuthTokenHash = &tokenHash

		if err := s.devices.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("updating device: %w", err)
		}

		return &RegisterResponse{
			DeviceID:              existing.ID.String(),
			AuthToken:             token,
			HeartbeatIntervalSecs: defaultHeartbeatSecs,
			TelemetryBatchSize:    defaultBatchSize,
			MaxCacheSizeMB:        defaultMaxCacheMB,
		}, nil
	}

	// New device
	d := &Device{
		Fingerprint:  req.Fingerprint,
		OS:           os,
		OSVersion:    req.OSVersion,
		RAMMB:        req.RAMMB,
		Chipset:      req.Chipset,
		HasNPU:       req.HasNPU,
		HasGPU:       req.HasGPU,
		NPUTflops:    req.NPUTflops,
		AgentVersion: req.AgentVersion,
		Labels:       req.Labels,
	}

	// Classify
	className := ClassifyDevice(d)
	dc := s.buildDeviceClass(d, className)
	if err := s.classes.Upsert(ctx, dc); err != nil {
		return nil, fmt.Errorf("upserting device class: %w", err)
	}
	d.DeviceClassID = &dc.ID

	if err := s.devices.Create(ctx, d); err != nil {
		return nil, fmt.Errorf("creating device: %w", err)
	}

	token, tokenHash, err := s.generateToken(d.ID, d.Fingerprint)
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}
	d.AuthTokenHash = &tokenHash
	// Update with token hash
	if err := s.devices.Update(ctx, d); err != nil {
		s.logger.Warn("failed to save auth token hash", "device_id", d.ID, "error", err)
	}

	return &RegisterResponse{
		DeviceID:              d.ID.String(),
		AuthToken:             token,
		HeartbeatIntervalSecs: defaultHeartbeatSecs,
		TelemetryBatchSize:    defaultBatchSize,
		MaxCacheSizeMB:        defaultMaxCacheMB,
	}, nil
}

// Heartbeat handles periodic device check-ins.
func (s *Service) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	dev, err := s.devices.GetByID(ctx, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("getting device: %w", err)
	}

	if err := s.devices.UpdateLastSeen(ctx, req.DeviceID); err != nil {
		s.logger.Warn("failed to update last_seen_at", "device_id", req.DeviceID, "error", err)
	}

	var assignments []ModelAssignment

	// Use intelligence orchestrator if available, fall back to legacy
	if s.assignmentComp != nil {
		assignments, err = s.assignmentComp.ComputeAssignments(ctx, dev, req.LoadedModels)
		if err != nil {
			s.logger.Warn("intelligence assignment failed, falling back to legacy", "device_id", req.DeviceID, "error", err)
			assignments, err = s.computeAssignments(ctx, dev, req.LoadedModels)
		}
	} else {
		assignments, err = s.computeAssignments(ctx, dev, req.LoadedModels)
	}
	if err != nil {
		s.logger.Warn("failed to compute assignments", "device_id", req.DeviceID, "error", err)
	}

	// Merge experiment assignments (experiment takes priority)
	if s.experimentComp != nil {
		expAssignments, expErr := s.experimentComp.ComputeExperimentAssignments(ctx, dev, req.LoadedModels)
		if expErr != nil {
			s.logger.Warn("experiment assignment failed", "device_id", req.DeviceID, "error", expErr)
		} else {
			assignments = append(assignments, expAssignments...)
		}
	}

	return &HeartbeatResponse{
		Assignments:          assignments,
		NextHeartbeatSeconds: defaultHeartbeatSecs,
	}, nil
}

func (s *Service) computeAssignments(ctx context.Context, device *Device, currentlyLoaded []ModelStatus) ([]ModelAssignment, error) {
	deployments, err := s.deployments.ListActiveForDevice(ctx, device.Labels)
	if err != nil {
		return nil, fmt.Errorf("listing active deployments: %w", err)
	}

	// Build set of loaded variant IDs
	loadedVariants := make(map[string]ModelStatus)
	for _, m := range currentlyLoaded {
		loadedVariants[m.VariantID] = m
	}

	// Track which variants should be active
	activeVariantIDs := make(map[string]bool)
	var assignments []ModelAssignment

	for _, dep := range deployments {
		// For canary deployments, check if device is in canary group
		if dep.Strategy == "CANARY" && dep.CanaryPct > 0 && dep.CanaryPct < 100 {
			if !isInCanary(device.ID, dep.ID, dep.CanaryPct) {
				continue
			}
		}

		variants, err := s.variants.ListByVersion(ctx, dep.ModelVersionID)
		if err != nil {
			s.logger.Warn("failed to list variants for deployment", "deployment_id", dep.ID, "error", err)
			continue
		}

		// Convert to registry types for variant selection
		regVariants := toRegistryVariants(variants)
		caps := toDeviceCapabilities(device)

		best, err := registry.SelectVariant(regVariants, caps)
		if err != nil {
			continue // No compatible variant for this device
		}

		activeVariantIDs[best.ID] = true

		// Check if already loaded
		if _, loaded := loadedVariants[best.ID]; loaded {
			continue // Already has the right variant
		}

		// Check if device has a different variant for same model — that's an UPDATE
		action := "LOAD"
		for _, loaded := range currentlyLoaded {
			if loaded.ModelID == dep.ModelVersionID.String() {
				action = "UPDATE"
				break
			}
		}

		downloadURL := ""
		// Find the VariantInfo to get blob key
		for _, v := range variants {
			if v.ID.String() == best.ID {
				downloadURL, _ = s.storage.GeneratePresignedDownload(ctx, v.BlobKey, downloadURLExpiry)
				break
			}
		}

		assignments = append(assignments, ModelAssignment{
			ModelID:       dep.ModelVersionID.String(),
			Version:       best.VersionID,
			VariantID:     best.ID,
			DownloadURL:   downloadURL,
			BlobSizeBytes: uint64(best.BlobSizeBytes),
			BlobSHA256:    best.BlobSHA256,
			Action:        action,
		})
	}

	// Check for models to unload (loaded but no active deployment)
	for variantID, loaded := range loadedVariants {
		if !activeVariantIDs[variantID] {
			assignments = append(assignments, ModelAssignment{
				ModelID:   loaded.ModelID,
				VariantID: variantID,
				Action:    "UNLOAD",
			})
		}
	}

	return assignments, nil
}

// ValidateToken validates a JWT token and returns the device_id claim.
func (s *Service) ValidateToken(tokenString string) (uuid.UUID, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.tokenSecret), nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return uuid.Nil, fmt.Errorf("invalid token claims")
	}

	deviceIDStr, ok := claims["device_id"].(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("missing device_id claim")
	}

	return uuid.Parse(deviceIDStr)
}

// GetDevice returns a device by ID.
func (s *Service) GetDevice(ctx context.Context, id uuid.UUID) (*Device, error) {
	return s.devices.GetByID(ctx, id)
}

// ListDevices returns a paginated list of devices.
func (s *Service) ListDevices(ctx context.Context, limit, offset int) ([]Device, int, error) {
	return s.devices.List(ctx, limit, offset)
}

// ListDevicesByLabels returns devices matching the given labels.
func (s *Service) ListDevicesByLabels(ctx context.Context, labels map[string]string, limit, offset int) ([]Device, int, error) {
	return s.devices.ListByLabels(ctx, labels, limit, offset)
}

// DeleteDevice deletes a device by ID.
func (s *Service) DeleteDevice(ctx context.Context, id uuid.UUID) error {
	return s.devices.Delete(ctx, id)
}

// ListDeviceClasses returns all device classes.
func (s *Service) ListDeviceClasses(ctx context.Context) ([]DeviceClass, error) {
	return s.classes.List(ctx)
}

// GetDeviceClass returns a device class by ID.
func (s *Service) GetDeviceClass(ctx context.Context, id uuid.UUID) (*DeviceClass, error) {
	return s.classes.GetByID(ctx, id)
}

// --- helpers ---

func (s *Service) generateToken(deviceID uuid.UUID, fingerprint string) (tokenStr, tokenHash string, err error) {
	claims := jwt.MapClaims{
		"device_id":   deviceID.String(),
		"fingerprint": fingerprint,
		"iat":         time.Now().Unix(),
		"exp":         time.Now().Add(tokenExpiry).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err = token.SignedString([]byte(s.tokenSecret))
	if err != nil {
		return "", "", fmt.Errorf("signing token: %w", err)
	}

	h := sha256.Sum256([]byte(tokenStr))
	tokenHash = fmt.Sprintf("%x", h)
	return tokenStr, tokenHash, nil
}

func (s *Service) buildDeviceClass(d *Device, className string) *DeviceClass {
	return &DeviceClass{
		Name:      className,
		OS:        d.OS,
		MinRAMMB:  d.RAMMB,
		HasNPU:    d.HasNPU,
		HasGPU:    d.HasGPU,
		NPUTflops: d.NPUTflops,
	}
}

func isInCanary(deviceID, deploymentID uuid.UUID, canaryPct int) bool {
	h := sha256.Sum256([]byte(deviceID.String() + deploymentID.String()))
	bucket := int(binary.BigEndian.Uint32(h[:4])) % 100
	return bucket < canaryPct
}

func toRegistryVariants(variants []VariantInfo) []*registry.ModelVariant {
	result := make([]*registry.ModelVariant, len(variants))
	for i, v := range variants {
		result[i] = &registry.ModelVariant{
			ID:            v.ID.String(),
			VersionID:     v.VersionID.String(),
			Quantization:  v.Quantization,
			TargetRuntime: v.TargetRuntime,
			MinRAMMB:      v.MinRAMMB,
			MinNPUTflops:  v.MinNPUTflops,
			BlobKey:       v.BlobKey,
			BlobSizeBytes: v.BlobSizeBytes,
			BlobSHA256:    v.BlobSHA256,
		}
	}
	return result
}

func toDeviceCapabilities(d *Device) registry.DeviceCapabilities {
	// Determine the runtime based on OS
	runtime := "CUSTOM"
	switch d.OS {
	case "ANDROID":
		runtime = "TFLITE"
	case "IOS":
		runtime = "COREML"
	case "LINUX":
		runtime = "ONNX"
	case "WINDOWS":
		runtime = "ONNX"
	}
	return registry.DeviceCapabilities{
		RAMMB:     d.RAMMB,
		HasNPU:    d.HasNPU,
		HasGPU:    d.HasGPU,
		NPUTflops: d.NPUTflops,
		Runtime:   runtime,
	}
}
