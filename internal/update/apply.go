package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// ChecksumsFileName is the digest list GoReleaser publishes with every release.
const ChecksumsFileName = "checksums.txt"

// projectName matches the archive name_template in .goreleaser.yaml.
const projectName = "sshforward"

// maxArchiveSize caps a download. The binary is a few megabytes; a response
// far beyond that is not a release archive.
const maxArchiveSize = 128 << 20

// AssetName returns the release archive published for a platform. It mirrors
// the archives name_template in .goreleaser.yaml, so the two must change
// together.
func AssetName(goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("%s_%s_%s%s", projectName, goos, goarch, ext)
}

// BinaryName returns the executable's name inside the archive.
func BinaryName(goos string) string {
	if goos == "windows" {
		return projectName + ".exe"
	}
	return projectName
}

// ApplyOptions configures a self-update.
type ApplyOptions struct {
	// Release is the release to install.
	Release *Release
	// TargetPath is the executable to replace.
	TargetPath string
	// HTTP is the client used for downloads; nil means a default one.
	HTTP *http.Client
	// GOOS and GOARCH select the asset; empty means the running platform.
	GOOS   string
	GOARCH string
}

// Apply downloads the release archive for the target platform, verifies it
// against the published checksums, and replaces the executable.
//
// The download is verified before anything on disk is touched: an archive that
// does not match its published digest never reaches the file the user runs.
func Apply(ctx context.Context, opts ApplyOptions) error {
	goos, goarch := opts.GOOS, opts.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	assetName := AssetName(goos, goarch)
	assetURL, ok := opts.Release.AssetURL(assetName)
	if !ok {
		return fmt.Errorf("release %s publishes no %s for %s/%s", opts.Release.Version, assetName, goos, goarch)
	}
	sumsURL, ok := opts.Release.AssetURL(ChecksumsFileName)
	if !ok {
		return fmt.Errorf("release %s publishes no %s, so the download cannot be verified", opts.Release.Version, ChecksumsFileName)
	}

	client := opts.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * DefaultTimeout}
	}

	sums, err := download(ctx, client, sumsURL)
	if err != nil {
		return fmt.Errorf("cannot download %s: %w", ChecksumsFileName, err)
	}
	want, err := checksumFor(string(sums), assetName)
	if err != nil {
		return err
	}

	archive, err := download(ctx, client, assetURL)
	if err != nil {
		return fmt.Errorf("cannot download %s: %w", assetName, err)
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s — refusing to install",
			assetName, hex.EncodeToString(got[:]), want)
	}

	binary, err := extractBinary(archive, assetName, BinaryName(goos))
	if err != nil {
		return err
	}
	return replaceExecutable(opts.TargetPath, binary)
}

// download fetches a URL into memory, bounded by maxArchiveSize.
func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxArchiveSize))
}

// checksumFor finds an asset's digest in a GoReleaser checksums.txt, whose
// lines are "<sha256>  <filename>".
func checksumFor(checksums, assetName string) (string, error) {
	for line := range strings.Lines(checksums) {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("%s lists no entry for %s", ChecksumsFileName, assetName)
}

// extractBinary pulls the executable out of a release archive.
func extractBinary(archive []byte, assetName, binaryName string) ([]byte, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractFromZip(archive, binaryName)
	}
	return extractFromTarGz(archive, binaryName)
}

func extractFromTarGz(archive []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("cannot read release archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read release archive: %w", err)
		}
		if path.Base(hdr.Name) != binaryName {
			continue
		}
		return io.ReadAll(io.LimitReader(tr, maxArchiveSize))
	}
	return nil, fmt.Errorf("release archive contains no %s", binaryName)
}

func extractFromZip(archive []byte, binaryName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("cannot read release archive: %w", err)
	}
	for _, f := range zr.File {
		if path.Base(f.Name) == binaryName {
			return readZipEntry(f, binaryName)
		}
	}
	return nil, fmt.Errorf("release archive contains no %s", binaryName)
}

func readZipEntry(f *zip.File, binaryName string) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("cannot read %s from the archive: %w", binaryName, err)
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxArchiveSize))
}

// replaceExecutable swaps target for the new binary through renames, which are
// atomic within a directory. The old file is moved aside rather than deleted:
// Windows refuses to delete a running image but allows renaming it, and the
// rename gives Unix a rollback if the second step fails.
func replaceExecutable(target string, binary []byte) error {
	dir := filepath.Dir(target)

	mode := os.FileMode(0o755)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, "."+projectName+"-new-*")
	if err != nil {
		return fmt.Errorf("cannot create the new binary next to %s: %w", target, err)
	}
	tmpName := tmp.Name()
	// A no-op once the rename below succeeds; cleanup if it does not.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot write the new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cannot write the new binary: %w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("cannot make the new binary executable: %w", err)
	}

	backup := target + ".old"
	_ = os.Remove(backup) // leftover from an earlier update on Windows
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("cannot move %s aside: %w", target, err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Rename(backup, target) // put the working binary back
		return fmt.Errorf("cannot install the new binary at %s: %w", target, err)
	}

	// Best-effort: Windows keeps the running image locked until the process
	// exits, so the leftover is cleared by the next update instead.
	_ = os.Remove(backup)
	return nil
}
