package device

import (
	"time"

	"github.com/google/uuid"
)

// Device represents a registered edge device.
type Device struct {
	ID            uuid.UUID
	ProjectID     *uuid.UUID // project the device enrolled into; nil for pre-enrollment devices
	DeviceClassID *uuid.UUID
	Fingerprint   string
	OS            string // ANDROID, IOS, LINUX, WINDOWS
	OSVersion     string
	RAMMB         int
	Chipset       string
	HasNPU        bool
	HasGPU        bool
	NPUTflops     float64
	AgentVersion  string
	Labels        map[string]string
	AuthTokenHash *string
	LastSeenAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// DeviceClass represents a category of devices with similar hardware.
type DeviceClass struct {
	ID            uuid.UUID
	Name          string
	OS            string
	MinRAMMB      int
	HasNPU        bool
	HasGPU        bool
	NPUTflops     float64
	ChipsetFamily *string
	CreatedAt     time.Time
}
