package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pcpl2lab/sshforward/internal/update"
	"gopkg.in/yaml.v3"
)

// publishScripts are the repository publishers, which share both their shape
// and the ways they can go quietly wrong.
var publishScripts = []string{
	"installer/apt/publish.sh",
	"installer/rpm/publish.sh",
	"installer/apk/publish.sh",
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

const issPath = "installer/sshforward.iss"

// appID is the Inno Setup AppId of the Windows installer. Windows recognises an
// upgrade of an existing install by this value, so changing it would install a
// second copy beside the first and orphan the old uninstall entry. It is
// pinned here so that a change has to be deliberate.
const appID = "{{923ADEC1-21DD-42F7-A149-0AAAB97049F9}"

func readISS(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(issPath)
	if err != nil {
		t.Fatalf("read %s: %v", issPath, err)
	}
	return string(data)
}

// issDirective returns the value of a [Setup] directive such as AppId.
func issDirective(script, name string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `=(.*)$`)
	m := re.FindStringSubmatch(script)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func TestInstallerAppIDIsPinned(t *testing.T) {
	if got := issDirective(readISS(t), "AppId"); got != appID {
		t.Errorf("got AppId %q, want %q; changing it breaks upgrades of existing installs", got, appID)
	}
}

func TestInstallerNeedsNoAdministrator(t *testing.T) {
	// The binary's own manifest declares asInvoker. An installer that demanded
	// elevation would contradict it and put a UAC prompt in front of a tool
	// that only ever writes inside the user's profile.
	if got := issDirective(readISS(t), "PrivilegesRequired"); got != "lowest" {
		t.Errorf("got PrivilegesRequired %q, want lowest", got)
	}
}

func TestInstallerReferencedFilesExist(t *testing.T) {
	// The script reaches out of its own directory for the licence, the readme
	// and the icon. Moving or renaming any of them breaks the release build,
	// which otherwise only shows up on the Windows runner.
	script := readISS(t)

	paths := map[string]string{
		"LicenseFile":   issDirective(script, "LicenseFile"),
		"SetupIconFile": issDirective(script, "SetupIconFile"),
	}
	for _, m := range regexp.MustCompile(`(?m)^Source:\s*"([^"]+)"`).FindAllStringSubmatch(script, -1) {
		src := m[1]
		// Architecture binaries come from BinDir, which only exists during a
		// release build.
		if strings.Contains(src, "{#BinDir}") {
			continue
		}
		paths["Source "+src] = src
	}

	for what, rel := range paths {
		if rel == "" {
			t.Errorf("%s is not set in %s", what, issPath)
			continue
		}
		// Paths in the script are relative to the script's own directory.
		full := filepath.Join("installer", filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/")))
		if _, err := os.Stat(full); err != nil {
			t.Errorf("%s points at %q, which does not exist: %v", what, rel, err)
		}
	}
}

func TestInstallerCoversEveryWindowsArch(t *testing.T) {
	// One installer serves all Windows architectures by picking a binary at
	// install time. An architecture that the build matrix produces but the
	// script does not reference would silently install nothing on that machine.
	data, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}
	var cfg goreleaserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}

	script := readISS(t)
	for _, arch := range cfg.windowsArches() {
		if !strings.Contains(script, `{#BinDir}\`+arch+`\sshforward.exe`) {
			t.Errorf("windows/%s is built but %s installs no binary for it", arch, issPath)
		}
	}
}

func TestMacOSPackageIdentifierIsConsistent(t *testing.T) {
	// The identifier appears in three places: the Go constant that finds the
	// receipt, the productbuild distribution, and the pkgbuild call in the
	// release workflow. If they drift, the package still installs but
	// `sshforward update` stops recognising it and offers to overwrite a file
	// the installer owns.
	id := update.MacOSPackageID

	for _, f := range []string{
		"installer/macos/distribution.xml",
		".github/workflows/release.yml",
	} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if !strings.Contains(string(data), id) {
			t.Errorf("%s does not mention the package identifier %q", f, id)
		}
	}
}

func TestMacOSInstallerShipsAUniversalBinary(t *testing.T) {
	// Two architectures fused into one binary is what lets the package install
	// without asking the user which Mac they have. Dropping the lipo step would
	// silently ship an Intel-only or ARM-only package.
	data, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	workflow := string(data)

	if !strings.Contains(workflow, "lipo -create") {
		t.Error("the macOS package job no longer fuses the two architectures with lipo")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if !strings.Contains(workflow, "macbin/"+arch+"/sshforward") {
			t.Errorf("the macOS package job does not use the %s binary", arch)
		}
	}
}

func TestRepositoryCachePolicies(t *testing.T) {
	// Every repository is served by Cloudflare Pages, which caches at the edge.
	// A cached index served next to freshly published packages is what produces
	// apt's "Hash Sum mismatch" and keeps a new release invisible to dnf and apk.
	tests := []struct {
		script string
		index  string
	}{
		{script: "installer/apt/publish.sh", index: "/dists/*"},
		{script: "installer/rpm/publish.sh", index: "/repodata/*"},
		{script: "installer/apk/publish.sh", index: "/%s/APKINDEX.tar.gz"},
	}

	for _, tt := range tests {
		t.Run(tt.script, func(t *testing.T) {
			// Some rules are written with a heredoc and some with printf, so
			// unescape the printf newlines to read both the same way.
			body := strings.ReplaceAll(readFile(t, tt.script), `\n`, "\n")

			if !strings.Contains(body, ">_headers") {
				t.Fatalf("%s writes no _headers file, so Cloudflare's default caching applies to the index", tt.script)
			}

			_, afterIndex, found := strings.Cut(body, tt.index)
			if !found {
				t.Fatalf("%s has no cache rule mentioning %s", tt.script, tt.index)
			}
			rule, _, _ := strings.Cut(strings.TrimLeft(afterIndex, "\n"), "\n\n")
			if !strings.Contains(rule, "no-cache") {
				t.Errorf("the index rule for %s is not no-cache; got %q", tt.index, rule)
			}
		})
	}
}

func TestCacheRulePatternsCanActuallyMatch(t *testing.T) {
	// Cloudflare allows one splat per pattern and matches it greedily, so a
	// pattern like "/*/APKINDEX.tar.gz" or "/*/*.apk" matches nothing at all.
	// A rule that never fires is indistinguishable from one that works, which
	// is how a four-hour cache once ended up on an index meant to be no-cache.
	for _, script := range publishScripts {
		// Unescape printf newlines so a rule built with printf reads the same
		// as one written in a heredoc.
		lines := strings.Split(strings.ReplaceAll(readFile(t, script), `\n`, "\n"), "\n")

		checked := 0
		for i, line := range lines {
			// A rule is a path followed by an indented header line. Looking for
			// that pair is what keeps unrelated paths in the script out of this.
			if i+1 >= len(lines) || !strings.Contains(lines[i+1], "Cache-Control:") {
				continue
			}
			// Drop any shell prefix, so `printf '/a/*` reads as `/a/*`.
			p := line
			if _, after, found := strings.Cut(p, "'"); found {
				p = after
			}
			p = strings.TrimSpace(p)
			if !strings.HasPrefix(p, "/") {
				continue
			}
			checked++

			if !strings.Contains(p, "*") {
				continue
			}
			if strings.Count(p, "*") > 1 {
				t.Errorf("%s: pattern %q has more than one splat, which Cloudflare rejects outright", script, p)
			}
			if !strings.HasSuffix(p, "*") {
				t.Errorf("%s: pattern %q puts a splat before the end, which silently matches nothing", script, p)
			}
		}

		if checked == 0 {
			t.Errorf("%s: found no cache rules to check, so this guard is not actually guarding anything", script)
		}
	}
}

func TestEveryRepositoryPublisherSignsItsIndex(t *testing.T) {
	// An unsigned index means clients must be told to disable verification,
	// which defeats signing the packages in the first place.
	for _, script := range publishScripts {
		body := readFile(t, script)
		// APT and RPM sign with PGP; Alpine uses RSA through abuild-sign.
		if !strings.Contains(body, "gpg --batch") && !strings.Contains(body, "abuild-sign") {
			t.Errorf("%s never signs anything", script)
		}
	}
}
