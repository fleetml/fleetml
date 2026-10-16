// Package version is the version of the FleetML binaries: the CLI
// (`fleetml version`) and the Linux agent (`fleetml-agent -version`, and the
// agent version it reports when it registers). Release builds set it from the
// tag (scripts/release-binaries.sh):
//
//	go build -ldflags "-X github.com/fleetml/fleetml/internal/version.Version=0.5.0"
package version

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Version is set at build time with -ldflags -X; "dev" otherwise.
var Version = "dev"

var release = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// String returns the version without a leading "v". A build without -X
// reports the module version Go recorded when it is a release version
// (`go install github.com/fleetml/fleetml/cmd/fleetml@v0.5.0`, or `go build`
// in a clean checkout of a tag), and "dev" for any other build.
func String() string {
	if Version != "dev" {
		return strings.TrimPrefix(Version, "v")
	}
	if bi, ok := debug.ReadBuildInfo(); ok && release.MatchString(bi.Main.Version) {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return "dev"
}
