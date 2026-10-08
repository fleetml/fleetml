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
