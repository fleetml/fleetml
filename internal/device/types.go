package device

import (
	"time"

	"github.com/google/uuid"
)

// Device represents a registered edge device.
type Device struct {
	ID            uuid.UUID         `json:"id"`
	ProjectID     *uuid.UUID        `json:"project_id"` // project the device enrolled into; nil for pre-enrollment devices
	DeviceClassID *uuid.UUID        `json:"device_class_id"`
	Fingerprint   string            `json:"-"`
	OS            string            `json:"os"` // ANDROID, IOS, LINUX, WINDOWS
	OSVersion     string            `json:"os_version"`
	RAMMB         int               `json:"ram_mb"`
	Chipset       string            `json:"chipset"`
	HasNPU        bool              `json:"has_npu"`
	HasGPU        bool              `json:"has_gpu"`
	NPUTflops     float64           `json:"npu_tflops"`
	AgentVersion  string            `json:"agent_version"`
	Labels        map[string]string `json:"labels"`
	AuthTokenHash *string           `json:"-"`
	LastSeenAt    *time.Time        `json:"last_seen_at"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// DeviceDetail is one device as GET /devices/:id shows it: the device plus
// what it reported about its models, so a device-side developer can tell
// whether a deployment's model actually arrived.
type DeviceDetail struct {
	Device
	// LoadedModels are the model versions the device's last heartbeat
	// reported loaded.
	LoadedModels []LoadedModel `json:"loaded_models"`
	// LastModelEvent is the newest MODEL_LOAD, MODEL_LOAD_FAILED or
	// SIGNATURE_INVALID event the device reported, if any.
	LastModelEvent *ModelEvent `json:"last_model_event"`
}

// LoadedModel is a model variant a device reports loaded.
type LoadedModel struct {
	Model        string    `json:"model"`
	Version      string    `json:"version"`
	VersionID    uuid.UUID `json:"version_id"`
	VariantID    uuid.UUID `json:"variant_id"`
	Quantization string    `json:"quantization"`
	Runtime      string    `json:"runtime"`
}

// ModelEvent is a model lifecycle event a device reported.
type ModelEvent struct {
	Type      string    `json:"type"`
	Model     string    `json:"model"`
	Version   string    `json:"version"`
	VersionID uuid.UUID `json:"version_id"`
	Error     string    `json:"error,omitempty"`
	At        time.Time `json:"at"`
}

// DeviceClass represents a category of devices with similar hardware.
type DeviceClass struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	OS            string    `json:"os"`
	MinRAMMB      int       `json:"min_ram_mb"`
	HasNPU        bool      `json:"has_npu"`
	HasGPU        bool      `json:"has_gpu"`
	NPUTflops     float64   `json:"npu_tflops"`
	ChipsetFamily *string   `json:"chipset_family"`
	CreatedAt     time.Time `json:"created_at"`
}

// ListFilter narrows a device list; its zero value lists every device.
type ListFilter struct {
	ProjectID *uuid.UUID        // only the devices enrolled into this project
	Labels    map[string]string // only the devices that carry all of these labels
}

// Matches reports whether d passes the filter (for in-memory repositories).
func (f ListFilter) Matches(d *Device) bool {
	if f.ProjectID != nil && (d.ProjectID == nil || *d.ProjectID != *f.ProjectID) {
		return false
	}
	for k, v := range f.Labels {
		if d.Labels[k] != v {
			return false
		}
	}
	return true
}
