package update

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SourceEnv lets a packager or a test state outright where the binary came
// from, overriding path detection.
const SourceEnv = "SSHFORWARD_INSTALL_SOURCE"

// Source is where the running binary was installed from. It decides whether
// sshforward may replace itself: overwriting a file a package manager owns
// breaks its manifest and is undone by the next upgrade.
type Source int

const (
	// SourceManual is a binary the user placed there — a downloaded release,
	// `go install`, or a local build. Only these may self-update.
	SourceManual Source = iota
	SourceHomebrew
	SourceScoop
	SourceWinGet
	SourceSystemPackage
	// SourceInstaller is a copy placed by the Windows installer. It lives in a
	// user-writable directory, so self-update could overwrite it - but that
	// would leave the uninstall entry advertising the old version, and the next
	// run of the installer would undo it anyway.
	SourceInstaller
)

func (s Source) String() string {
	switch s {
	case SourceHomebrew:
		return "homebrew"
	case SourceScoop:
		return "scoop"
	case SourceWinGet:
		return "winget"
	case SourceSystemPackage:
		return "system package"
	case SourceInstaller:
		return "the Windows installer"
	default:
		return "manual"
	}
}

// Managed reports whether a package manager owns this binary.
func (s Source) Managed() bool {
	return s != SourceManual
}

// UpgradeCommand returns the command that updates a managed install, or an
// empty string for a manual one.
func (s Source) UpgradeCommand() string {
	switch s {
	case SourceHomebrew:
		return "brew upgrade sshforward"
	case SourceScoop:
		return "scoop update sshforward"
	case SourceWinGet:
		return "winget upgrade sshforward"
	case SourceSystemPackage:
		return "sudo apt update && sudo apt upgrade sshforward"
	case SourceInstaller:
		return "download and run the latest installer from " + ReleasesURL
	default:
		return ""
	}
}

// ParseSource reads a Source from the name used by SourceEnv.
func ParseSource(s string) (Source, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "homebrew", "brew":
		return SourceHomebrew, true
	case "scoop":
		return SourceScoop, true
	case "winget":
		return SourceWinGet, true
	case "deb", "apt", "rpm", "system":
		return SourceSystemPackage, true
	case "installer", "innosetup":
		return SourceInstaller, true
	case "manual":
		return SourceManual, true
	default:
		return SourceManual, false
	}
}

// DetectSource classifies the running binary. Symlinks are resolved first, so
// a Homebrew shim in /usr/local/bin is recognised by the Cellar path behind it.
func DetectSource() Source {
	if s, ok := ParseSource(os.Getenv(SourceEnv)); ok {
		return s
	}
	exe, err := os.Executable()
	if err != nil {
		return SourceManual
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return detectSourceForExecutable(exe, runtime.GOOS)
}

// MacOSPackageID is the identifier of the macOS installer package. It must
// match the one productbuild is given in the release workflow, or an installed
// package goes unrecognised.
const MacOSPackageID = "ovh.pcpl2lab.sshforward"

// macOSInstallDir is where the package puts the binary. It is on the default
// PATH, which is why the package uses it.
const macOSInstallDir = "/usr/local/bin"

// macOSReceiptPath is where macOS records that our package is installed. It is
// a variable so tests can point it at a temporary file.
var macOSReceiptPath = "/var/db/receipts/" + MacOSPackageID + ".plist"

// detectSourceForExecutable classifies a resolved executable, consulting the
// filesystem for markers that a path alone cannot reveal.
func detectSourceForExecutable(exe, goos string) Source {
	if s := detectSourceForPath(exe, goos); s != SourceManual {
		return s
	}

	switch goos {
	case "windows":
		// Inno Setup always writes its uninstaller next to the program, which
		// identifies the install wherever the user put it.
		if hasInnoUninstaller(filepath.Dir(exe)) {
			return SourceInstaller
		}
	case "darwin":
		// The receipt only says the package is installed somewhere. A copy the
		// user placed elsewhere is still theirs to replace, so the binary has
		// to be the one the package owns.
		p := strings.ReplaceAll(exe, `\`, "/")
		if strings.HasPrefix(p, macOSInstallDir+"/") && fileExists(macOSReceiptPath) {
			return SourceInstaller
		}
	}
	return SourceManual
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// hasInnoUninstaller reports whether dir holds an Inno Setup uninstaller.
func hasInnoUninstaller(dir string) bool {
	// Inno numbers them when several installs share a directory: unins000.exe,
	// unins001.exe, and so on.
	matches, err := filepath.Glob(filepath.Join(dir, "unins[0-9][0-9][0-9].exe"))
	return err == nil && len(matches) > 0
}

// detectSourceForPath is the pure part of DetectSource, so the path rules can
// be tested for every platform from any platform.
func detectSourceForPath(path, goos string) Source {
	p := strings.ReplaceAll(path, `\`, "/")
	if goos == "windows" {
		p = strings.ToLower(p)
	}

	switch {
	case strings.Contains(p, "/scoop/apps/"):
		return SourceScoop
	case strings.Contains(p, "/microsoft/winget/"):
		return SourceWinGet
	case strings.Contains(p, "/cellar/") || strings.Contains(p, "/Cellar/"):
		return SourceHomebrew
	case strings.Contains(p, "/opt/homebrew/") || strings.Contains(p, "/.linuxbrew/"):
		return SourceHomebrew
	}

	// dpkg and rpm own /usr/bin and /usr/sbin. They deliberately do not own
	// /usr/local, which is reserved for whatever the administrator puts there.
	if goos != "windows" && (strings.HasPrefix(p, "/usr/bin/") || strings.HasPrefix(p, "/usr/sbin/")) {
		return SourceSystemPackage
	}
	return SourceManual
}
