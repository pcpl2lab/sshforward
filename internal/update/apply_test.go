package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{goos: "linux", goarch: "amd64", want: "sshforward_linux_amd64.tar.gz"},
		{goos: "linux", goarch: "arm64", want: "sshforward_linux_arm64.tar.gz"},
		{goos: "darwin", goarch: "arm64", want: "sshforward_darwin_arm64.tar.gz"},
		{goos: "windows", goarch: "amd64", want: "sshforward_windows_amd64.zip"},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"_"+tt.goarch, func(t *testing.T) {
			if got := AssetName(tt.goos, tt.goarch); got != tt.want {
				t.Errorf("AssetName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

func TestBinaryName(t *testing.T) {
	if got := BinaryName("linux"); got != "sshforward" {
		t.Errorf("BinaryName(linux) = %q, want sshforward", got)
	}
	if got := BinaryName("windows"); got != "sshforward.exe" {
		t.Errorf("BinaryName(windows) = %q, want sshforward.exe", got)
	}
}

// tarGz builds a gzipped tar holding one file, the way GoReleaser ships one.
func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// zipArchive builds a zip holding one file.
func zipArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// releaseServer serves an archive and a checksums.txt, and returns a Release
// pointing at them. checksumOverride, when non-empty, is published instead of
// the archive's real digest so a mismatch can be exercised.
func releaseServer(t *testing.T, assetName string, archive []byte, checksumOverride string) (*Release, *httptest.Server) {
	t.Helper()

	sum := sha256Hex(archive)
	if checksumOverride != "" {
		sum = checksumOverride
	}
	checksums := fmt.Sprintf("%s  %s\n%s  sshforward_other_arch.tar.gz\n", sum, assetName, strings.Repeat("0", 64))

	mux := http.NewServeMux()
	mux.HandleFunc("/archive", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &Release{
		Version: "v1.4.0",
		Assets: []Asset{
			{Name: assetName, URL: srv.URL + "/archive"},
			{Name: ChecksumsFileName, URL: srv.URL + "/checksums.txt"},
		},
	}, srv
}

func writeTarget(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sshforward")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	return path
}

func TestApply_ReplacesBinaryFromTarGz(t *testing.T) {
	const newBinary = "#!/bin/sh\necho new version\n"
	assetName := AssetName("linux", "amd64")
	rel, _ := releaseServer(t, assetName, tarGz(t, "sshforward", []byte(newBinary)), "")
	target := writeTarget(t, "old version")

	err := Apply(context.Background(), ApplyOptions{
		Release: rel, TargetPath: target, GOOS: "linux", GOARCH: "amd64",
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if string(got) != newBinary {
		t.Errorf("got binary content %q, want %q", got, newBinary)
	}
}

func TestApply_ReplacesBinaryFromZip(t *testing.T) {
	const newBinary = "MZ new windows binary"
	assetName := AssetName("windows", "amd64")
	rel, _ := releaseServer(t, assetName, zipArchive(t, "sshforward.exe", []byte(newBinary)), "")
	target := writeTarget(t, "old version")

	err := Apply(context.Background(), ApplyOptions{
		Release: rel, TargetPath: target, GOOS: "windows", GOARCH: "amd64",
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if string(got) != newBinary {
		t.Errorf("got binary content %q, want %q", got, newBinary)
	}
}

func TestApply_ChecksumMismatchLeavesOriginalUntouched(t *testing.T) {
	// The one test that matters most: a tampered or truncated download must
	// never reach the file the user runs with access to their SSH keys.
	const original = "old version"
	assetName := AssetName("linux", "amd64")
	rel, _ := releaseServer(t, assetName, tarGz(t, "sshforward", []byte("malicious payload")), strings.Repeat("a", 64))
	target := writeTarget(t, original)

	err := Apply(context.Background(), ApplyOptions{
		Release: rel, TargetPath: target, GOOS: "linux", GOARCH: "amd64",
	})
	if err == nil {
		t.Fatal("got nil error for a checksum mismatch, want the update to be refused")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("got error %q, want it to name the checksum mismatch", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != original {
		t.Errorf("target was modified to %q despite the mismatch, want it left as %q", got, original)
	}
}

func TestApply_MissingPlatformAssetIsReported(t *testing.T) {
	rel, _ := releaseServer(t, AssetName("linux", "amd64"), tarGz(t, "sshforward", []byte("x")), "")
	target := writeTarget(t, "old version")

	err := Apply(context.Background(), ApplyOptions{
		Release: rel, TargetPath: target, GOOS: "plan9", GOARCH: "mips",
	})
	if err == nil {
		t.Fatal("got nil error for a platform with no release asset, want one")
	}
	if !strings.Contains(err.Error(), AssetName("plan9", "mips")) {
		t.Errorf("got error %q, want it to name the missing asset", err)
	}
}

func TestApply_MissingChecksumsAssetIsReported(t *testing.T) {
	assetName := AssetName("linux", "amd64")
	rel, srv := releaseServer(t, assetName, tarGz(t, "sshforward", []byte("x")), "")
	rel.Assets = []Asset{{Name: assetName, URL: srv.URL + "/archive"}} // checksums.txt withheld
	target := writeTarget(t, "old version")

	err := Apply(context.Background(), ApplyOptions{
		Release: rel, TargetPath: target, GOOS: "linux", GOARCH: "amd64",
	})
	if err == nil {
		t.Fatal("got nil error when the release publishes no checksums, want the update refused")
	}
	if !strings.Contains(err.Error(), ChecksumsFileName) {
		t.Errorf("got error %q, want it to name the missing checksums file", err)
	}
}

func TestApply_ArchiveWithoutBinaryIsReported(t *testing.T) {
	assetName := AssetName("linux", "amd64")
	rel, _ := releaseServer(t, assetName, tarGz(t, "README.md", []byte("no binary here")), "")
	target := writeTarget(t, "old version")

	err := Apply(context.Background(), ApplyOptions{
		Release: rel, TargetPath: target, GOOS: "linux", GOARCH: "amd64",
	})
	if err == nil {
		t.Fatal("got nil error for an archive with no binary in it, want one")
	}

	got, _ := os.ReadFile(target)
	if string(got) != "old version" {
		t.Errorf("target was modified to %q, want it left untouched", got)
	}
}

const applyHelperEnv = "GO_SSHFORWARD_APPLY_HELPER"

// TestHelperSleeper is not a test: it is the body of the child process started
// by TestReplaceExecutable_WhileRunning.
func TestHelperSleeper(t *testing.T) {
	if os.Getenv(applyHelperEnv) != "1" {
		t.Skip("helper process body")
	}
	time.Sleep(2 * time.Minute)
}

func TestReplaceExecutable_WhileRunning(t *testing.T) {
	// The riskiest step of a self-update is overwriting the image of a process
	// that is still running — Windows refuses to delete one, which is why the
	// old file is renamed aside rather than removed.
	self, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Skipf("cannot read the test binary: %v", err)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "running"+exeSuffix())
	if err := os.WriteFile(target, self, 0o755); err != nil {
		t.Fatalf("copy test binary: %v", err)
	}

	cmd := exec.Command(target, "-test.run=TestHelperSleeper", "--")
	cmd.Env = append(os.Environ(), applyHelperEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the copied binary: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	const replacement = "replacement binary"
	if err := replaceExecutable(target, []byte(replacement)); err != nil {
		t.Fatalf("replaceExecutable on a running image failed: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if string(got) != replacement {
		t.Errorf("got %q at the target path, want the replacement content", got)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func TestAssetName_StaysInSyncWithGoreleaser(t *testing.T) {
	// AssetName reconstructs the archive names GoReleaser publishes. If the
	// release config drifts, self-update starts asking for files that do not
	// exist — and it would only be noticed after a release.
	data, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}
	config := string(data)

	for _, want := range []string{
		"{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}", // AssetName's shape
		"formats: [tar.gz]",                        // default archive format
		"formats: [zip]",                           // windows override
	} {
		if !strings.Contains(config, want) {
			t.Errorf(".goreleaser.yaml no longer contains %q; AssetName must be updated to match", want)
		}
	}
}
