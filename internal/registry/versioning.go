package registry

import (
	"fmt"
	"strconv"
	"strings"
)

type Semver struct {
	Major int
	Minor int
	Patch int
}

func (s Semver) String() string {
	return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
}

func (s Semver) LessThan(other Semver) bool {
	if s.Major != other.Major {
		return s.Major < other.Major
	}
	if s.Minor != other.Minor {
		return s.Minor < other.Minor
	}
	return s.Patch < other.Patch
}

// ParseSemver reads a semver 2.0.0 version (an optional "v" first): MAJOR.MINOR.PATCH
// without leading zeros, then an optional -prerelease and +build, which are
// checked but not kept in the Semver (the version string is stored as given).
func ParseSemver(given string) (Semver, error) {
	version := strings.TrimPrefix(given, "v")
	version, build, hasBuild := strings.Cut(version, "+")
	core, pre, hasPre := strings.Cut(version, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Semver{}, fmt.Errorf("invalid semver %q: expected 3 parts, got %d", given, len(parts))
	}
	var nums [3]int
	for i, name := range []string{"major", "minor", "patch"} {
		n, ok := semverNumber(parts[i])
		if !ok {
			return Semver{}, fmt.Errorf("invalid semver %q: bad %s version", given, name)
		}
		nums[i] = n
	}
	if hasPre && !semverIdentifiers(pre, true) {
		return Semver{}, fmt.Errorf("invalid semver %q: bad prerelease (dot-separated [0-9A-Za-z-], numbers without leading zeros)", given)
	}
	if hasBuild && !semverIdentifiers(build, false) {
		return Semver{}, fmt.Errorf("invalid semver %q: bad build metadata (dot-separated [0-9A-Za-z-])", given)
	}
	return Semver{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil
}

// semverNumber is a number of digits only, without a leading zero.
func semverNumber(s string) (int, bool) {
	if s == "" || strings.Trim(s, "0123456789") != "" || len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// semverIdentifiers checks dot-separated identifiers of [0-9A-Za-z-]; in a
// prerelease, an all-digit identifier has no leading zero.
func semverIdentifiers(s string, prerelease bool) bool {
	for _, id := range strings.Split(s, ".") {
		if id == "" || strings.Trim(id, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-") != "" {
			return false
		}
		if prerelease && strings.Trim(id, "0123456789") == "" && len(id) > 1 && id[0] == '0' {
			return false
		}
	}
	return true
}

func ValidateSemver(version string) error {
	_, err := ParseSemver(version)
	return err
}

func NextPatch(version string) (string, error) {
	s, err := ParseSemver(version)
	if err != nil {
		return "", err
	}
	s.Patch++
	return s.String(), nil
}

func NextMinor(version string) (string, error) {
	s, err := ParseSemver(version)
	if err != nil {
		return "", err
	}
	s.Minor++
	s.Patch = 0
	return s.String(), nil
}

func NextMajor(version string) (string, error) {
	s, err := ParseSemver(version)
	if err != nil {
		return "", err
	}
	s.Major++
	s.Minor = 0
	s.Patch = 0
	return s.String(), nil
}
