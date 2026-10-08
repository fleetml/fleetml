package agent

import (
	"time"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

// MayDownload applies the download policy (DeviceConfig.download_policy) the
// same way as the SDKs: urgent assignments need only a connection.
func MayDownload(policy fleetmlv1.DownloadPolicy, urgent, metered, onBattery bool) bool {
	if urgent {
		return true
	}
	switch policy {
	case fleetmlv1.DownloadPolicy_ANY_NETWORK:
		return true
	case fleetmlv1.DownloadPolicy_UNMETERED_ONLY:
		return !metered
	default: // UNMETERED_OR_CHARGING, and the agent's default when the server leaves it unspecified
		return !metered || !onBattery
	}
}

// failedCooldown is how long a variant that failed validation is not retried.
const failedCooldown = time.Hour

// refreshDue reports whether the device token should be refreshed: 6 days
// before expiry, or at half its lifetime for short-lived tokens (as the SDKs).
func refreshDue(now, issued, expires time.Time) bool {
	if expires.IsZero() {
		return false
	}
	window := 6 * 24 * time.Hour
	if !issued.IsZero() && issued.Before(expires) {
		if half := expires.Sub(issued) / 2; half < window {
			window = half
		}
	}
	return expires.Sub(now) <= window
}
