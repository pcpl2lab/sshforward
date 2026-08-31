package cmd

import "testing"

func TestBackgroundCheckAllowed(t *testing.T) {
	tests := []struct {
		name           string
		command        string
		currentVersion string
		envDisable     string
		configEnabled  bool
		want           bool
	}{
		{
			name:    "released build running a normal command",
			command: "list", currentVersion: "v1.0.0", configEnabled: true, want: true,
		},
		{
			name:    "development build is never nagged",
			command: "list", currentVersion: devVersion, configEnabled: true, want: false,
		},
		{
			name:    "environment opt-out wins",
			command: "list", currentVersion: "v1.0.0", envDisable: "1", configEnabled: true, want: false,
		},
		{
			name:    "any non-empty value opts out",
			command: "list", currentVersion: "v1.0.0", envDisable: "0", configEnabled: true, want: false,
		},
		{
			name:    "config opt-out is honoured",
			command: "list", currentVersion: "v1.0.0", configEnabled: false, want: false,
		},
		{
			name:    "version command reports on its own",
			command: "version", currentVersion: "v1.0.0", configEnabled: true, want: false,
		},
		{
			name:    "update command reports on its own",
			command: "update", currentVersion: "v1.0.0", configEnabled: true, want: false,
		},
		{
			name:    "help must stay instant and offline",
			command: "help", currentVersion: "v1.0.0", configEnabled: true, want: false,
		},
		{
			name:    "completion output must never gain an extra line",
			command: "completion", currentVersion: "v1.0.0", configEnabled: true, want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := backgroundCheckAllowed(tt.command, tt.currentVersion, tt.envDisable, tt.configEnabled)
			if got != tt.want {
				t.Errorf("backgroundCheckAllowed(%q, %q, %q, %v) = %v, want %v",
					tt.command, tt.currentVersion, tt.envDisable, tt.configEnabled, got, tt.want)
			}
		})
	}
}
