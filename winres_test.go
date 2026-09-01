package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// winresConfig mirrors the parts of winres/winres.json this project relies on.
type winresConfig struct {
	GroupIcon map[string]map[string][]string `json:"RT_GROUP_ICON"`
	Manifest  map[string]map[string]struct {
		ExecutionLevel string `json:"execution-level"`
		LongPathAware  bool   `json:"long-path-aware"`
	} `json:"RT_MANIFEST"`
	Version map[string]map[string]struct {
		Info map[string]map[string]string `json:"info"`
	} `json:"RT_VERSION"`
}

func loadWinres(t *testing.T) winresConfig {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("winres", "winres.json"))
	if err != nil {
		t.Fatalf("read winres/winres.json: %v", err)
	}
	var cfg winresConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse winres/winres.json: %v", err)
	}
	return cfg
}

// versionInfo returns the single language block of RT_VERSION.
func versionInfo(t *testing.T, cfg winresConfig) map[string]string {
	t.Helper()
	for _, langs := range cfg.Version {
		for _, block := range langs {
			for _, info := range block.Info {
				return info
			}
		}
	}
	t.Fatal("winres.json declares no RT_VERSION info block")
	return nil
}

func TestWinresVersionInfoIsComplete(t *testing.T) {
	// Windows shows these in a file's Properties dialog, and installers and
	// package managers read them. A blank field looks like an unfinished build.
	info := versionInfo(t, loadWinres(t))

	required := []string{
		"CompanyName",
		"Comments",
		"FileDescription",
		"InternalName",
		"LegalCopyright",
		"OriginalFilename",
		"ProductName",
	}
	for _, field := range required {
		if strings.TrimSpace(info[field]) == "" {
			t.Errorf("winres.json leaves %s empty; Windows Properties would show a blank field", field)
		}
	}

	if got := info["OriginalFilename"]; got != "sshforward.exe" {
		t.Errorf("got OriginalFilename %q, want sshforward.exe", got)
	}
	if got := info["ProductName"]; got != "sshforward" {
		t.Errorf("got ProductName %q, want sshforward", got)
	}

	// The version strings are filled in from the git tag at release time, so
	// hardcoding them here would ship a stale number.
	for _, field := range []string{"FileVersion", "ProductVersion"} {
		if info[field] != "" {
			t.Errorf("winres.json hardcodes %s = %q; it must be left to the release hook", field, info[field])
		}
	}
}

func TestWinresManifestKeepsTheBinaryUnprivileged(t *testing.T) {
	// A port forwarder must never trigger a UAC prompt: it binds unprivileged
	// ports and writes only inside the user's home directory.
	cfg := loadWinres(t)
	for _, langs := range cfg.Manifest {
		for _, m := range langs {
			if m.ExecutionLevel != "as invoker" {
				t.Errorf("got execution-level %q, want \"as invoker\"", m.ExecutionLevel)
			}
			if !m.LongPathAware {
				t.Error("long-path-aware is off; paths beyond 260 characters would fail")
			}
			return
		}
	}
	t.Fatal("winres.json declares no RT_MANIFEST")
}

func TestWinresIconFilesExist(t *testing.T) {
	cfg := loadWinres(t)
	found := false
	for _, langs := range cfg.GroupIcon {
		for _, files := range langs {
			for _, name := range files {
				found = true
				if _, err := os.Stat(filepath.Join("winres", name)); err != nil {
					t.Errorf("winres.json references icon %q which is missing: %v", name, err)
				}
			}
		}
	}
	if !found {
		t.Error("winres.json declares no icon; the binary would fall back to the default console icon")
	}
}

// goreleaserConfig mirrors the parts of .goreleaser.yaml this test inspects.
type goreleaserConfig struct {
	Before struct {
		Hooks []string `yaml:"hooks"`
	} `yaml:"before"`
	Builds []struct {
		Goos   []string `yaml:"goos"`
		Goarch []string `yaml:"goarch"`
		Ignore []struct {
			Goos   string `yaml:"goos"`
			Goarch string `yaml:"goarch"`
		} `yaml:"ignore"`
	} `yaml:"builds"`
}

// windowsArches returns the architectures actually built for Windows, which is
// the goarch list minus the combinations the config ignores.
func (c goreleaserConfig) windowsArches() []string {
	if len(c.Builds) == 0 {
		return nil
	}
	build := c.Builds[0]

	ignored := map[string]bool{}
	for _, ig := range build.Ignore {
		if ig.Goos == "windows" {
			ignored[ig.Goarch] = true
		}
	}

	var out []string
	for _, arch := range build.Goarch {
		if !ignored[arch] {
			out = append(out, arch)
		}
	}
	return out
}

func TestGoreleaserGeneratesResourcesForEveryWindowsArch(t *testing.T) {
	// The .syso files are per-architecture and are not committed. An arch that
	// the build produces but the hook does not cover ships with no metadata at
	// all, and nothing would notice until someone opened Properties.
	data, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}
	var cfg goreleaserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}
	if len(cfg.Builds) == 0 {
		t.Fatal(".goreleaser.yaml declares no builds")
	}

	var hook string
	for _, h := range cfg.Before.Hooks {
		if strings.Contains(h, "go-winres") {
			hook = h
			break
		}
	}
	if hook == "" {
		t.Fatal(".goreleaser.yaml has no go-winres hook; Windows builds would carry no version resource")
	}

	covered := archList(hook)
	built := cfg.windowsArches()
	if len(built) == 0 {
		t.Fatal("no Windows architectures resolved from .goreleaser.yaml")
	}
	for _, arch := range built {
		if !covered[arch] {
			t.Errorf("windows/%s is built but the go-winres hook does not cover it (--arch %v)", arch, keys(covered))
		}
	}

	// The reverse direction matters too: an --arch entry for a target that is
	// no longer built wastes a build step and hides a stale config.
	for arch := range covered {
		if !slices.Contains(built, arch) {
			t.Errorf("the go-winres hook builds a resource for windows/%s, which is not in the build matrix", arch)
		}
	}
}

func TestEveryGoWinresInvocationSetsTheVersion(t *testing.T) {
	// go-winres emits the version string table only when both version flags are
	// given: without them CompanyName, ProductName and the rest are silently
	// dropped and the binary ships with an empty Properties dialog, while the
	// resource still looks present because the fixed 0.0.0.0 block remains.
	data, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}
	var cfg goreleaserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}

	mainGo, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	invocations := map[string]string{}
	for _, h := range cfg.Before.Hooks {
		if strings.Contains(h, "go-winres") {
			invocations[".goreleaser.yaml hook"] = h
		}
	}
	for line := range strings.Lines(string(mainGo)) {
		if strings.Contains(line, "go:generate") && strings.Contains(line, "go-winres") {
			invocations["main.go go:generate"] = line
		}
	}
	if len(invocations) != 2 {
		t.Fatalf("found %d go-winres invocations, want the goreleaser hook and the go:generate line: %v", len(invocations), invocations)
	}

	for where, cmd := range invocations {
		for _, flag := range []string{"--file-version", "--product-version"} {
			if !strings.Contains(cmd, flag) {
				t.Errorf("%s omits %s, so the generated resource would carry no metadata strings", where, flag)
			}
		}
	}
}

// archList reads the value of the hook's --arch flag.
func archList(hook string) map[string]bool {
	out := map[string]bool{}
	fields := strings.Fields(hook)
	for i, f := range fields {
		if f == "--arch" && i+1 < len(fields) {
			for a := range strings.SplitSeq(fields[i+1], ",") {
				out[strings.TrimSpace(a)] = true
			}
		}
	}
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
