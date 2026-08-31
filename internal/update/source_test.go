package update

import "testing"

func TestDetectSourceForPath(t *testing.T) {
	tests := []struct {
		name string
		goos string
		path string
		want Source
	}{
		{
			name: "apple silicon homebrew prefix",
			goos: "darwin",
			path: "/opt/homebrew/bin/sshforward",
			want: SourceHomebrew,
		},
		{
			name: "intel homebrew cellar",
			goos: "darwin",
			path: "/usr/local/Cellar/sshforward/1.4.0/bin/sshforward",
			want: SourceHomebrew,
		},
		{
			name: "linuxbrew prefix",
			goos: "linux",
			path: "/home/linuxbrew/.linuxbrew/bin/sshforward",
			want: SourceHomebrew,
		},
		{
			name: "scoop apps directory",
			goos: "windows",
			path: `C:\Users\dev\scoop\apps\sshforward\current\sshforward.exe`,
			want: SourceScoop,
		},
		{
			name: "winget packages directory",
			goos: "windows",
			path: `C:\Users\dev\AppData\Local\Microsoft\WinGet\Packages\pcpl2.sshforward\sshforward.exe`,
			want: SourceWinGet,
		},
		{
			name: "winget path casing is ignored",
			goos: "windows",
			path: `c:\users\dev\appdata\local\microsoft\winget\packages\pcpl2.sshforward\sshforward.exe`,
			want: SourceWinGet,
		},
		{
			name: "usr bin belongs to the system package manager",
			goos: "linux",
			path: "/usr/bin/sshforward",
			want: SourceSystemPackage,
		},
		{
			name: "usr local bin is not owned by dpkg",
			goos: "linux",
			path: "/usr/local/bin/sshforward",
			want: SourceManual,
		},
		{
			name: "home directory install is manual",
			goos: "linux",
			path: "/home/dev/go/bin/sshforward",
			want: SourceManual,
		},
		{
			name: "arbitrary windows directory is manual",
			goos: "windows",
			path: `C:\tools\sshforward.exe`,
			want: SourceManual,
		},
		{
			name: "unix style separators on windows still match scoop",
			goos: "windows",
			path: "C:/Users/dev/scoop/apps/sshforward/current/sshforward.exe",
			want: SourceScoop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectSourceForPath(tt.path, tt.goos); got != tt.want {
				t.Errorf("detectSourceForPath(%q, %q) = %v, want %v", tt.path, tt.goos, got, tt.want)
			}
		})
	}
}

func TestSourceManaged(t *testing.T) {
	managed := []Source{SourceHomebrew, SourceScoop, SourceWinGet, SourceSystemPackage}
	for _, s := range managed {
		if !s.Managed() {
			t.Errorf("%v must be reported as managed; self-update would fight the package manager", s)
		}
		if s.UpgradeCommand() == "" {
			t.Errorf("%v is managed but offers no upgrade command to tell the user about", s)
		}
	}

	if SourceManual.Managed() {
		t.Error("a manually installed binary must not be reported as managed")
	}
	if SourceManual.UpgradeCommand() != "" {
		t.Error("a manual install has no package manager command to suggest")
	}
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		in   string
		want Source
		ok   bool
	}{
		{in: "homebrew", want: SourceHomebrew, ok: true},
		{in: "brew", want: SourceHomebrew, ok: true},
		{in: "scoop", want: SourceScoop, ok: true},
		{in: "winget", want: SourceWinGet, ok: true},
		{in: "deb", want: SourceSystemPackage, ok: true},
		{in: "apt", want: SourceSystemPackage, ok: true},
		{in: "manual", want: SourceManual, ok: true},
		{in: "  Manual  ", want: SourceManual, ok: true},
		{in: "chocolatey", ok: false},
		{in: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := ParseSource(tt.in)
			if ok != tt.ok {
				t.Fatalf("ParseSource(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("ParseSource(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
