package version

import "testing"

func TestString(t *testing.T) {
	defer func(v string) { Version = v }(Version)
	for in, want := range map[string]string{"0.5.0": "0.5.0", "v0.5.0": "0.5.0", "v1.2.3-rc.1": "1.2.3-rc.1"} {
		Version = in
		if got := String(); got != want {
			t.Errorf("Version %q: String() = %q, want %q", in, got, want)
		}
	}
	// A test binary records no release version.
	Version = "dev"
	if got := String(); got != "dev" {
		t.Errorf("String() = %q, want dev", got)
	}
}
