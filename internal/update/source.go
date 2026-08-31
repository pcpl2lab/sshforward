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
	return detectSourceForPath(exe, runtime.GOOS)
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
