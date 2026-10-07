package experiment

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/google/uuid"
)

// AssignCohort deterministically assigns a device to a cohort.
// Returns (cohort, included). If included is false, the device is excluded.
func AssignCohort(deviceID, experimentID uuid.UUID, splitRatio, trafficPercent int) (cohort string, included bool) {
	// Check if device is in the experiment
	trafficHash := sha256Sum(deviceID.String() + experimentID.String() + "traffic")
	trafficPct := int(binary.BigEndian.Uint32(trafficHash[:4]) % 100)
	if trafficPct >= trafficPercent {
		return "", false
	}

	// Assign to control or treatment
	cohortHash := sha256Sum(deviceID.String() + experimentID.String())
	cohortPct := int(binary.BigEndian.Uint32(cohortHash[:4]) % 100)
	if cohortPct < splitRatio {
		return "treatment", true
	}
	return "control", true
}

func sha256Sum(data string) [32]byte {
	return sha256.Sum256([]byte(data))
}
