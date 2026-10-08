package device

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/auth"
)

const (
	// DefaultTokenTTL is the lifetime of a device token.
	DefaultTokenTTL = 30 * 24 * time.Hour
	// RefreshWindow is how close to expiry a token must be before RefreshToken
	// issues a new one.
	RefreshWindow        = 7 * 24 * time.Hour
	defaultHeartbeatSecs = 60
	defaultBatchSize     = 100
	defaultFlushSecs     = 30
	defaultMaxCacheMB    = 512
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
	// EnrollmentKey is required for new devices.
	EnrollmentKey string
	// CurrentToken is the device's current token, if it has one (re-registration).
	CurrentToken string
}

// RegisterResponse is the service-layer response for device registration.
type RegisterResponse struct {
	DeviceID              string
	AuthToken             string
	TokenExpiresAt        time.Time
	HeartbeatIntervalSecs uint32
	TelemetryBatchSize    uint32
	TelemetryFlushSecs    uint32
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
	ModelID       string // the model (stable across versions)
	VersionID     string // the model version this assignment delivers
	ModelName     string
	Version       string // human-readable version string, e.g. "1.2.0"
	VariantID     string
	DownloadURL   string
	BlobSizeBytes uint64
	BlobSHA256    string
	Action        string // LOAD, UPDATE, UNLOAD
}

// ExperimentAssignment tells the device which experiment arm it is in. The
// device tags telemetry for that model version with ArmID.
type ExperimentAssignment struct {
	ExperimentID   string
	ArmName        string // "control" or "treatment"
	ArmID          string
	ModelVersionID string
}

// ExperimentPlan is what running experiments require of one device.
type ExperimentPlan struct {
	// Assignments are LOAD/UPDATE actions for arm models not yet loaded.
	Assignments []ModelAssignment
	// Keep holds the variant IDs of the device's arm models; they must not be unloaded.
	Keep map[string]bool
	// Models holds the model IDs an experiment controls on this device;
	// deployments must not change them while the experiment runs.
	Models      map[string]bool
	Experiments []ExperimentAssignment
}

// AssignmentComputer computes model assignments for a device.
// Implemented by the intelligence orchestrator.
type AssignmentComputer interface {
	ComputeAssignments(ctx context.Context, dev *Device, currentlyLoaded []ModelStatus) ([]ModelAssignment, error)
}

// ExperimentAssigner plans experiment arms for a device.
type ExperimentAssigner interface {
	PlanExperiments(ctx context.Context, dev *Device, currentlyLoaded []ModelStatus) (*ExperimentPlan, error)
}

// EnrollmentVerifier checks a per-project enrollment key and returns its project.
type EnrollmentVerifier interface {
	VerifyEnrollmentKey(ctx context.Context, key string) (uuid.UUID, error)
}

// Service orchestrates business logic for device management.
type Service struct {
	devices        DeviceRepository
	classes        DeviceClassRepository
	assignmentComp AssignmentComputer
	experimentComp ExperimentAssigner
	enrollment     EnrollmentVerifier
	tokenSecret    string
	tokenTTL       time.Duration
	logger         *slog.Logger
}

// NewService creates a new device service.
func NewService(
	devices DeviceRepository,
	classes DeviceClassRepository,
	tokenSecret string,
	logger *slog.Logger,
) *Service {
	return &Service{
		devices:     devices,
		classes:     classes,
		tokenSecret: tokenSecret,
		tokenTTL:    DefaultTokenTTL,
		logger:      logger,
	}
}

// SetAssignmentComputer sets the assignment computer (intelligence orchestrator).
func (s *Service) SetAssignmentComputer(ac AssignmentComputer) {
	s.assignmentComp = ac
}

// SetTokenTTL sets the lifetime of newly issued device tokens.
func (s *Service) SetTokenTTL(ttl time.Duration) {
	if ttl > 0 {
		s.tokenTTL = ttl
	}
}

// SetEnrollmentVerifier sets the enrollment key verifier. Without one, new
// devices cannot register.
func (s *Service) SetEnrollmentVerifier(v EnrollmentVerifier) {
	s.enrollment = v
}

// SetExperimentAssigner sets the experiment assigner.
func (s *Service) SetExperimentAssigner(ea ExperimentAssigner) {
	s.experimentComp = ea
}

// RegisterDevice handles first-time device registration or re-registration.
//
// A new device must present a valid enrollment key and is enrolled into that
// key's project. A known fingerprint may only re-register with its current
// device token or a valid enrollment key for its project; it then receives a
// new token and the previous one stops working.
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
		if err := s.authorizeReRegister(ctx, existing, req); err != nil {
			return nil, err
		}

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

		// Generate new token; storing its hash invalidates the previous one.
		token, tokenHash, expiresAt, err := s.generateToken(existing.ID, existing.Fingerprint)
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
			TokenExpiresAt:        expiresAt,
			HeartbeatIntervalSecs: defaultHeartbeatSecs,
			TelemetryBatchSize:    defaultBatchSize,
			TelemetryFlushSecs:    defaultFlushSecs,
			MaxCacheSizeMB:        defaultMaxCacheMB,
		}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("looking up device: %w", err)
	}

	// New device: enrollment key required.
	projectID, err := s.verifyEnrollment(ctx, req.EnrollmentKey)
	if err != nil {
		return nil, err
	}

	d := &Device{
		ProjectID:    &projectID,
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

	token, tokenHash, expiresAt, err := s.generateToken(d.ID, d.Fingerprint)
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}
	d.AuthTokenHash = &tokenHash
	// Update with token hash. Without it the token cannot be validated, so fail.
	if err := s.devices.Update(ctx, d); err != nil {
		return nil, fmt.Errorf("saving auth token hash: %w", err)
	}

	return &RegisterResponse{
		DeviceID:              d.ID.String(),
		AuthToken:             token,
		TokenExpiresAt:        expiresAt,
		HeartbeatIntervalSecs: defaultHeartbeatSecs,
		TelemetryBatchSize:    defaultBatchSize,
		TelemetryFlushSecs:    defaultFlushSecs,
		MaxCacheSizeMB:        defaultMaxCacheMB,
	}, nil
}

// verifyEnrollment returns the project of a valid enrollment key, or ErrUnauthenticated.
func (s *Service) verifyEnrollment(ctx context.Context, key string) (uuid.UUID, error) {
	if key == "" {
		return uuid.Nil, fmt.Errorf("%w: enrollment_key is required to register a new device", ErrUnauthenticated)
	}
	if s.enrollment == nil {
		return uuid.Nil, fmt.Errorf("%w: device enrollment is not configured on this server", ErrUnauthenticated)
	}
	projectID, err := s.enrollment.VerifyEnrollmentKey(ctx, key)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidKey) {
			return uuid.Nil, fmt.Errorf("%w: invalid or revoked enrollment key", ErrUnauthenticated)
		}
		return uuid.Nil, fmt.Errorf("verifying enrollment key: %w", err)
	}
	return projectID, nil
}

// authorizeReRegister allows a known fingerprint to re-register only with its
// current token, or with an enrollment key for the device's project (any
// project for devices registered before enrollment keys existed).
func (s *Service) authorizeReRegister(ctx context.Context, dev *Device, req *RegisterRequest) error {
	if req.CurrentToken != "" && s.tokenMatchesDevice(dev, req.CurrentToken) {
		return nil
	}
	if req.EnrollmentKey != "" {
		projectID, err := s.verifyEnrollment(ctx, req.EnrollmentKey)
		if err != nil {
			return err
		}
		if dev.ProjectID == nil {
			dev.ProjectID = &projectID
			return nil
		}
		if *dev.ProjectID == projectID {
			return nil
		}
	}
	return fmt.Errorf("%w: this device is already registered; re-register with its current device token or an enrollment key for its project", ErrUnauthenticated)
}

// tokenMatchesDevice reports whether token is a valid, unexpired token issued
// to dev and is the one currently stored for it.
func (s *Service) tokenMatchesDevice(dev *Device, token string) bool {
	claims, err := s.parseToken(token)
	if err != nil || claims.DeviceID != dev.ID || dev.AuthTokenHash == nil {
		return false
	}
	return auth.HashEqual(auth.HashKey(token), *dev.AuthTokenHash)
}

// Heartbeat handles periodic device check-ins.
func (s *Service) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	dev, err := s.devices.GetByID(ctx, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("getting device: %w", err)
	}

	loaded := make([]string, 0, len(req.LoadedModels))
	for _, m := range req.LoadedModels {
		if m.VariantID != "" {
			loaded = append(loaded, m.VariantID)
		}
	}
	if err := s.devices.RecordHeartbeat(ctx, req.DeviceID, loaded); err != nil {
		s.logger.Warn("failed to record heartbeat", "device_id", req.DeviceID, "error", err)
	}

	var assignments []ModelAssignment
	if s.assignmentComp == nil {
		s.logger.Warn("no assignment computer configured", "device_id", req.DeviceID)
	} else {
		assignments, err = s.assignmentComp.ComputeAssignments(ctx, dev, req.LoadedModels)
		if err != nil {
			// No assignments this round: the device keeps what it has and retries next heartbeat.
			s.logger.Warn("failed to compute assignments", "device_id", req.DeviceID, "error", err)
			assignments = nil
		}
	}

	var experiments []ExperimentAssignment
	if s.experimentComp != nil {
		plan, expErr := s.experimentComp.PlanExperiments(ctx, dev, req.LoadedModels)
		if expErr != nil {
			s.logger.Warn("experiment assignment failed", "device_id", req.DeviceID, "error", expErr)
		} else if plan != nil {
			assignments = mergeExperimentPlan(assignments, plan, req.LoadedModels)
			experiments = plan.Experiments
		}
	}

	return &HeartbeatResponse{
		Assignments:          assignments,
		Experiments:          experiments,
		NextHeartbeatSeconds: defaultHeartbeatSecs,
	}, nil
}

// mergeExperimentPlan gives running experiments priority over deployments:
// deployment actions for a model an experiment controls are dropped, the
// arm's variant is never unloaded, and once the arm's variant is loaded any
// other loaded variant of that model (e.g. the deployed version) is unloaded.
func mergeExperimentPlan(deployed []ModelAssignment, plan *ExperimentPlan, loaded []ModelStatus) []ModelAssignment {
	out := make([]ModelAssignment, 0, len(deployed)+len(plan.Assignments))
	unloading := map[string]bool{}
	for _, a := range deployed {
		if a.Action == "UNLOAD" {
			if plan.Keep[a.VariantID] {
				continue
			}
			unloading[a.VariantID] = true
		} else if plan.Models[a.ModelID] {
			continue
		}
		out = append(out, a)
	}
	out = append(out, plan.Assignments...)
	armLoaded := map[string]bool{}
	for _, m := range loaded {
		if plan.Keep[m.VariantID] {
			armLoaded[m.ModelID] = true
		}
	}
	for _, m := range loaded {
		if armLoaded[m.ModelID] && !plan.Keep[m.VariantID] && !unloading[m.VariantID] {
			out = append(out, ModelAssignment{ModelID: m.ModelID, VariantID: m.VariantID, Action: "UNLOAD"})
			unloading[m.VariantID] = true
		}
	}
	return out
}

// ValidateToken checks a device token's signature and expiry, and that it is
// the token currently stored for that device, so re-registration and
// revocation take effect immediately. Credential problems wrap
// ErrUnauthenticated; anything else is an internal error.
func (s *Service) ValidateToken(ctx context.Context, tokenString string) (deviceID uuid.UUID, expiresAt time.Time, err error) {
	claims, err := s.parseToken(tokenString)
	if err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	dev, err := s.devices.GetByID(ctx, claims.DeviceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return uuid.Nil, time.Time{}, fmt.Errorf("%w: device no longer exists", ErrUnauthenticated)
		}
		return uuid.Nil, time.Time{}, fmt.Errorf("loading device for token check: %w", err)
	}
	if dev.AuthTokenHash == nil || !auth.HashEqual(auth.HashKey(tokenString), *dev.AuthTokenHash) {
		return uuid.Nil, time.Time{}, fmt.Errorf("%w: token was revoked or superseded", ErrUnauthenticated)
	}
	return claims.DeviceID, claims.ExpiresAt, nil
}

// RefreshToken issues a new token for a device whose current token (already
// validated by the caller) expires within RefreshWindow. The old token stops
// working once the new hash is stored.
func (s *Service) RefreshToken(ctx context.Context, deviceID uuid.UUID, currentExpiry time.Time) (token string, expiresAt time.Time, err error) {
	if remaining := time.Until(currentExpiry); remaining > RefreshWindow {
		return "", time.Time{}, fmt.Errorf("%w: token is valid for another %s; refresh within %s of expiry",
			ErrRefreshTooEarly, remaining.Round(time.Hour), RefreshWindow)
	}
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("loading device: %w", err)
	}
	token, hash, expiresAt, err := s.generateToken(dev.ID, dev.Fingerprint)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generating token: %w", err)
	}
	if err := s.devices.SetAuthTokenHash(ctx, dev.ID, &hash); err != nil {
		return "", time.Time{}, fmt.Errorf("storing token hash: %w", err)
	}
	return token, expiresAt, nil
}

// RevokeDeviceToken invalidates a device's current token. The device must
// re-register with an enrollment key to get a new one.
func (s *Service) RevokeDeviceToken(ctx context.Context, deviceID uuid.UUID) error {
	return s.devices.SetAuthTokenHash(ctx, deviceID, nil)
}

// tokenClaims are the parts of a device token the server relies on.
type tokenClaims struct {
	DeviceID  uuid.UUID
	ExpiresAt time.Time
}

// parseToken checks the signature and expiry and returns the claims.
func (s *Service) parseToken(tokenString string) (tokenClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.tokenSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return tokenClaims{}, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return tokenClaims{}, fmt.Errorf("invalid token claims")
	}

	deviceIDStr, ok := claims["device_id"].(string)
	if !ok {
		return tokenClaims{}, fmt.Errorf("missing device_id claim")
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		return tokenClaims{}, fmt.Errorf("invalid device_id claim: %w", err)
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return tokenClaims{}, fmt.Errorf("missing exp claim")
	}
	return tokenClaims{DeviceID: deviceID, ExpiresAt: exp.Time}, nil
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

func (s *Service) generateToken(deviceID uuid.UUID, fingerprint string) (tokenStr, tokenHash string, expiresAt time.Time, err error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"device_id":   deviceID.String(),
		"fingerprint": fingerprint,
		"jti":         uuid.NewString(), // two tokens issued in the same second must differ
		"iat":         now.Unix(),
		"exp":         now.Add(s.tokenTTL).Unix(),
	}
	expiresAt = time.Unix(now.Add(s.tokenTTL).Unix(), 0)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err = token.SignedString([]byte(s.tokenSecret))
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("signing token: %w", err)
	}

	return tokenStr, auth.HashKey(tokenStr), expiresAt, nil
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
