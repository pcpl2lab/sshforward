package update

import (
	"os"
	"path/filepath"
	"testing"
)

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
			path: `C:\Users\dev\AppData\Local\Microsoft\WinGet\Packages\pcpl2lab.sshforward\sshforward.exe`,
			want: SourceWinGet,
		},
		{
			name: "winget path casing is ignored",
			goos: "windows",
			path: `c:\users\dev\appdata\local\microsoft\winget\packages\pcpl2lab.sshforward\sshforward.exe`,
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

func TestDetectSourceForExecutable_InnoSetupInstall(t *testing.T) {
	// Inno Setup always drops its uninstaller beside the program. That marker is
	// far more reliable than the install path, which is an ordinary-looking
	// directory under %LOCALAPPDATA%\Programs and would otherwise read as a
	// manual install that self-update is free to overwrite.
	dir := t.TempDir()
	exe := filepath.Join(dir, "sshforward.exe")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	if got := detectSourceForExecutable(exe, "windows"); got != SourceManual {
		t.Errorf("without the uninstaller present, got %v, want %v", got, SourceManual)
	}

	if err := os.WriteFile(filepath.Join(dir, "unins000.exe"), []byte("uninstaller"), 0o755); err != nil {
		t.Fatalf("write uninstaller: %v", err)
	}
	if got := detectSourceForExecutable(exe, "windows"); got != SourceInstaller {
		t.Errorf("with the uninstaller beside it, got %v, want %v", got, SourceInstaller)
	}
}

func TestDetectSourceForExecutable_UninstallerIsWindowsOnly(t *testing.T) {
	// A file called unins000.exe on Linux means nothing.
	dir := t.TempDir()
	exe := filepath.Join(dir, "sshforward")
	for _, name := range []string{"sshforward", "unins000.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if got := detectSourceForExecutable(exe, "linux"); got != SourceManual {
		t.Errorf("got %v on linux, want %v", got, SourceManual)
	}
}

func TestDetectSourceForExecutable_PathRulesStillWin(t *testing.T) {
	// A package manager path must not be reclassified by the marker check.
	got := detectSourceForExecutable(`C:\Users\dev\scoop\apps\sshforward\current\sshforward.exe`, "windows")
	if got != SourceScoop {
		t.Errorf("got %v, want %v", got, SourceScoop)
	}
}

func TestSourceInstallerIsManagedAndAdvises(t *testing.T) {
	if !SourceInstaller.Managed() {
		t.Error("an installer-managed copy must not be replaced by self-update")
	}
	if SourceInstaller.UpgradeCommand() == "" {
		t.Error("SourceInstaller must tell the user how to get the new version")
	}
	if _, ok := ParseSource("installer"); !ok {
		t.Error(`ParseSource("installer") must be recognised for the env override`)
	}
}

func TestDetectSourceForExecutable_MacOSPackageReceipt(t *testing.T) {
	// A .pkg install lands in /usr/local/bin, which dpkg-style rules treat as a
	// manual install. macOS records the package in /var/db/receipts, and that
	// receipt is what tells the two apart.
	receipt := filepath.Join(t.TempDir(), "ovh.pcpl2lab.sshforward.plist")
	previous := macOSReceiptPath
	macOSReceiptPath = receipt
	t.Cleanup(func() { macOSReceiptPath = previous })

	const installed = "/usr/local/bin/sshforward"

	if got := detectSourceForExecutable(installed, "darwin"); got != SourceManual {
		t.Errorf("with no receipt, got %v, want %v", got, SourceManual)
	}

	if err := os.WriteFile(receipt, []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	if got := detectSourceForExecutable(installed, "darwin"); got != SourceInstaller {
		t.Errorf("with the receipt present, got %v, want %v", got, SourceInstaller)
	}

	// A copy the user put somewhere else is theirs to replace, even while the
	// package is installed.
	if got := detectSourceForExecutable("/Users/dev/bin/sshforward", "darwin"); got != SourceManual {
		t.Errorf("for a binary outside the install location, got %v, want %v", got, SourceManual)
	}

	// The receipt is a macOS concept and must not leak into other platforms.
	if got := detectSourceForExecutable(installed, "linux"); got != SourceManual {
		t.Errorf("on linux, got %v, want %v", got, SourceManual)
	}
}
