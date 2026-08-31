package cmd

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// Injected at build time via -ldflags "-X github.com/pcpl2/sshforward/cmd.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// devVersion is what version holds when no build injected one.
const devVersion = "dev"

// resolveVersion picks the version to report. A release build injects it
// through ldflags; a `go install pkg@v1.2.3` build does not, but the module
// version it recorded is just as authoritative, and reporting it lets those
// binaries take part in the update check instead of being stuck at "dev".
//
// Only a real released tag counts. The toolchain also stamps builds from a
// working tree or an untagged commit, as "(devel)", a "+dirty" suffix or a VCS
// pseudo-version — none of which correspond to a published release, so
// comparing them against one would nag a developer about their own build.
func resolveVersion(ldflagsVersion, buildInfoVersion string) string {
	if ldflagsVersion != devVersion {
		return ldflagsVersion
	}
	if buildInfoVersion == "" || buildInfoVersion == "(devel)" {
		return devVersion
	}
	if strings.Contains(buildInfoVersion, "+") {
		return devVersion
	}
	if !semver.IsValid(buildInfoVersion) || module.IsPseudoVersion(buildInfoVersion) {
		return devVersion
	}
	return buildInfoVersion
}

// mainModuleVersion returns the version the toolchain stamped into the binary.
func mainModuleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		fmt.Fprintf(cmd.OutOrStdout(), "sshforward %s\ncommit: %s\nbuilt:  %s\nos/arch: %s/%s\n",
			version, commit, date, runtime.GOOS, runtime.GOARCH)
		return nil
	},
}

func init() {
	version = resolveVersion(version, mainModuleVersion())
	rootCmd.Version = version // enables `sshforward --version`
	rootCmd.AddCommand(versionCmd)
}
