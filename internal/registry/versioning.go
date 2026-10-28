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

func ParseSemver(given string) (Semver, error) {
	version := strings.TrimPrefix(given, "v")
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return Semver{}, fmt.Errorf("invalid semver %q: expected 3 parts, got %d", given, len(parts))
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return Semver{}, fmt.Errorf("invalid semver %q: bad major version", given)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return Semver{}, fmt.Errorf("invalid semver %q: bad minor version", given)
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil || patch < 0 {
		return Semver{}, fmt.Errorf("invalid semver %q: bad patch version", given)
	}

	return Semver{Major: major, Minor: minor, Patch: patch}, nil
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
