package update

import "testing"

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "patch bump is newer", current: "v1.0.0", latest: "v1.0.1", want: true},
		{name: "minor bump is newer", current: "v1.0.0", latest: "v1.1.0", want: true},
		{name: "major bump is newer", current: "v1.9.9", latest: "v2.0.0", want: true},
		{name: "same version is not newer", current: "v1.0.0", latest: "v1.0.0", want: false},
		{name: "older release is not newer", current: "v1.0.1", latest: "v1.0.0", want: false},
		{name: "missing v prefix is tolerated", current: "1.0.0", latest: "1.0.1", want: true},
		{name: "mixed prefixes are tolerated", current: "1.0.0", latest: "v1.0.1", want: true},

		// A locally built binary has no meaningful version; nagging it would be noise.
		{name: "dev build never reports an update", current: "dev", latest: "v9.9.9", want: false},
		{name: "go install devel marker never reports an update", current: "(devel)", latest: "v9.9.9", want: false},

		// Prereleases are opt-in: you only see them if you already run one.
		{name: "stable does not offer a prerelease", current: "v1.0.0", latest: "v1.1.0-rc.1", want: false},
		{name: "prerelease offers a newer prerelease", current: "v1.1.0-rc.1", latest: "v1.1.0-rc.2", want: true},
		{name: "prerelease offers the final release", current: "v1.1.0-rc.1", latest: "v1.1.0", want: true},

		{name: "unparsable latest is ignored", current: "v1.0.0", latest: "not-a-version", want: false},
		{name: "empty latest is ignored", current: "v1.0.0", latest: "", want: false},
		{name: "unparsable current is ignored", current: "custom-build", latest: "v1.0.0", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNewer(tt.current, tt.latest); got != tt.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
