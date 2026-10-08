// Package rollout holds the consistent-hash ring shared by canary and rolling
// deployments, so the server's exposure decisions and its health statistics
// agree on which devices are in a cohort.
package rollout

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"github.com/google/uuid"
)

// RingSize is the number of positions on the ring.
const RingSize = int64(math.MaxUint32) + 1

// Position is a device's fixed position on a deployment's ring. Different
// deployments shuffle the fleet differently, but a device keeps its position
// for the life of one deployment, so cohorts only ever grow.
func Position(deviceID, deploymentID uuid.UUID) uint32 {
	h := sha256.Sum256([]byte(deviceID.String() + deploymentID.String()))
	return binary.BigEndian.Uint32(h[:4])
}

// Bucket maps a device to 0..99 for percentage-based canary stages.
func Bucket(deviceID, deploymentID uuid.UUID) int {
	return int(Position(deviceID, deploymentID) % 100)
}

// InCanary reports whether a device is inside a canary of pct percent.
func InCanary(deviceID, deploymentID uuid.UUID, pct int) bool {
	return Bucket(deviceID, deploymentID) < pct
}

// PercentThreshold converts a percentage into a ring threshold: devices with
// Position below it are exposed. 100 or more exposes the whole ring.
func PercentThreshold(pct int) int64 {
	if pct >= 100 {
		return RingSize
	}
	if pct <= 0 {
		return 0
	}
	return RingSize * int64(pct) / 100
}

// Exposed reports whether a device's position is below a ring threshold.
func Exposed(deviceID, deploymentID uuid.UUID, threshold int64) bool {
	return int64(Position(deviceID, deploymentID)) < threshold
}
