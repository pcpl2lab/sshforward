package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"version"})
	defer rootCmd.SetArgs(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "sshforward") {
		t.Errorf("version output should contain program name, got: %q", out)
	}
	if !strings.Contains(out, version) {
		t.Errorf("version output should contain version %q, got: %q", version, out)
	}
}

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name      string
		ldflags   string
		buildInfo string
		want      string
	}{
		{
			name:      "release build uses the injected version",
			ldflags:   "v1.2.3",
			buildInfo: "v1.0.0",
			want:      "v1.2.3",
		},
		{
			name:      "go install falls back to the module version",
			ldflags:   "dev",
			buildInfo: "v1.2.3",
			want:      "v1.2.3",
		},
		{
			name:      "local build stays dev",
			ldflags:   "dev",
			buildInfo: "(devel)",
			want:      "dev",
		},
		{
			name:      "no build info at all stays dev",
			ldflags:   "dev",
			buildInfo: "",
			want:      "dev",
		},
		{
			// `go build` in a working tree stamps a VCS pseudo-version. It is
			// not a published release and must not be compared against one.
			name:      "vcs pseudo-version from a dirty tree stays dev",
			ldflags:   "dev",
			buildInfo: "v0.0.0-20260831112207-1a319f3905a3+dirty",
			want:      "dev",
		},
		{
			name:      "vcs pseudo-version from a clean tree stays dev",
			ldflags:   "dev",
			buildInfo: "v0.0.0-20260831112207-1a319f3905a3",
			want:      "dev",
		},
		{
			name:      "build metadata marks a working-tree build",
			ldflags:   "dev",
			buildInfo: "v1.2.3+dirty",
			want:      "dev",
		},
		{
			name:      "unparsable build info stays dev",
			ldflags:   "dev",
			buildInfo: "garbage",
			want:      "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.ldflags, tt.buildInfo); got != tt.want {
				t.Errorf("resolveVersion(%q, %q) = %q, want %q", tt.ldflags, tt.buildInfo, got, tt.want)
			}
		})
	}
}
