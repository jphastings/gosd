// Package artifacts downloads and caches the CI-built board artifacts
// (kernels, device trees, bootloaders) GoSD compiles itself and publishes as
// GitHub Releases tagged artifacts/vX.Y.Z (see .github/workflows/build-
// artifacts.yml and bean gosd-wtpa).
//
// Unlike internal/fetch, which pins one file at a time by URL+sha256, a
// release here is a whole per-board tarball whose contents are described by
// a manifest.json published alongside it. EnsureBoard downloads that
// manifest and tarball once, verifies every file the manifest lists for the
// requested board against its sha256, and caches the result under
// cacheDir/<Version>/<board>/ so every subsequent call — for any board —
// works without touching the network again, as long as the cache still
// verifies.
package artifacts

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"

	"github.com/jphastings/gosd/internal/fetch"
)

// Version pins the artifact release this build of gosd downloads: the
// GitHub Release published from the git tag "artifacts/<Version>". Bumping
// this constant to track a new artifacts/vX.Y.Z release (cut per
// docs/artifacts.md) is the only step needed to move gosd onto newer
// CI-built kernels/U-Boot.
//
// v0.3.0 carries the DTS patch enabling header/FPC I2C on the Rockchip
// boards (bean gosd-85pt): rk3566-radxa-zero-3e.dtb now has i2c3 enabled,
// and rk3528-nanopi-zero2.dtb has i2c5 enabled (plus its alias). The Pi
// boards' I2C is config.txt-only and needed no artifact change.
const Version = "v0.3.0"

// repoSlug is the GitHub repository artifact releases are published to.
const repoSlug = "jphastings/gosd"

// Manifest is the top-level manifest.json a build-artifacts.yml run
// publishes alongside the per-board tarballs, matching gosd-wtpa's locked
// schema: {version, boards: {<name>: {files: [{name, sha256, size}]}}}.
type Manifest struct {
	Version string                `json:"version"`
	Boards  map[string]BoardFiles `json:"boards"`
}

// BoardFiles is one board's entry in Manifest.
type BoardFiles struct {
	// Source records, per compiled component (e.g. "kernel", "uboot"),
	// the upstream repo/commit/config path it was built from — carried
	// through for GPL provenance, not consulted by EnsureBoard itself.
	Source map[string]ComponentSource `json:"source,omitempty"`
	Files  []FileEntry                `json:"files"`
}

// ComponentSource records where one compiled component's source came from.
type ComponentSource struct {
	Repo   string `json:"repo"`
	Ref    string `json:"ref"`
	Config string `json:"config"`
}

// FileEntry pins one extracted file's expected name, digest, and size.
type FileEntry struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// EnsureBoard ensures board's artifacts for Version are present and
// sha256-verified under cacheDir/Version/board, downloading and extracting
// them from this repository's GitHub Release the first time it's called for
// a given cacheDir/board/Version combination, and returns that directory.
//
// A nil client uses http.DefaultClient. Later calls with the same cacheDir
// make no network request at all, provided the cached files still verify —
// gosd build works fully offline after the first successful call.
func EnsureBoard(ctx context.Context, client *http.Client, cacheDir, board string) (string, error) {
	return ensureBoard(ctx, client, cacheDir, board, Version, func(name string) string {
		return fmt.Sprintf("https://github.com/%s/releases/download/artifacts/%s/%s", repoSlug, Version, name)
	})
}

// ensureBoard is EnsureBoard's testable core: urlFor maps a release asset
// name ("manifest.json" or "<board>.tar.zst") to a download URL, so tests
// can point it at an httptest.Server instead of GitHub.
func ensureBoard(ctx context.Context, client *http.Client, cacheDir, board, version string, urlFor func(name string) string) (string, error) {
	if board == "" {
		return "", errors.New("artifacts: board name is required")
	}
	if client == nil {
		client = http.DefaultClient
	}

	versionDir := filepath.Join(cacheDir, version)
	boardDir := filepath.Join(versionDir, board)
	manifestPath := filepath.Join(versionDir, "manifest.json")

	if m, err := readManifestCache(manifestPath); err == nil {
		if bf, ok := m.Boards[board]; ok && verifyFiles(boardDir, bf.Files) == nil {
			return boardDir, nil // fully offline: cache already verified, no network touched
		}
	}

	manifest, err := fetchManifest(ctx, client, urlFor("manifest.json"))
	if err != nil {
		return "", fmt.Errorf(
			"downloading the gosd artifact manifest for release artifacts/%s failed: %w; "+
				"if you're offline, %s's artifacts must already be cached at %s from a previous "+
				"successful build, or supply them via --artifacts-dir",
			version, err, board, boardDir)
	}

	bf, ok := manifest.Boards[board]
	if !ok {
		return "", fmt.Errorf(
			"the artifacts/%s release manifest has no entry for board %q; known boards: %s",
			version, board, strings.Join(boardNames(manifest), ", "))
	}

	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return "", fmt.Errorf("creating artifact cache directory %s: %w", versionDir, err)
	}
	tmpDir, err := os.MkdirTemp(versionDir, board+".part-*")
	if err != nil {
		return "", fmt.Errorf("creating a temporary directory in %s: %w", versionDir, err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() // no-op once the rename below succeeds

	if err := fetchTarball(ctx, client, urlFor(board+".tar.zst"), tmpDir); err != nil {
		return "", fmt.Errorf("downloading %s's artifact tarball for release artifacts/%s failed: %w", board, version, err)
	}

	if err := verifyFiles(tmpDir, bf.Files); err != nil {
		return "", fmt.Errorf(
			"%s's downloaded artifacts for release artifacts/%s failed verification: %w "+
				"(the upstream release may be corrupt; re-run to retry, or report this)",
			board, version, err)
	}

	if err := os.RemoveAll(boardDir); err != nil {
		return "", fmt.Errorf("clearing stale cache directory %s: %w", boardDir, err)
	}
	if err := os.Rename(tmpDir, boardDir); err != nil {
		return "", fmt.Errorf("moving downloaded artifacts into place at %s: %w", boardDir, err)
	}

	if err := writeManifestCache(manifestPath, manifest); err != nil {
		return "", fmt.Errorf("caching artifact manifest at %s: %w", manifestPath, err)
	}

	return boardDir, nil
}

func boardNames(m Manifest) []string {
	names := make([]string, 0, len(m.Boards))
	for name := range m.Boards {
		names = append(names, name)
	}
	return names
}

// readManifestCache reads and parses a previously-cached manifest.json.
func readManifestCache(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing cached manifest %s: %w", path, err)
	}
	return m, nil
}

// writeManifestCache persists manifest so future EnsureBoard calls (for this
// or any other board at the same version) can verify their cache without a
// network request.
func writeManifestCache(path string, manifest Manifest) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encoding manifest: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// verifyFiles checks that every file's sha256 and size match the manifest's
// pinned expectations, returning the first mismatch found. An empty files
// list always verifies.
func verifyFiles(dir string, files []FileEntry) error {
	for _, f := range files {
		path := filepath.Join(dir, f.Name)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		if info.Size() != f.Size {
			return fmt.Errorf("%s: size is %d bytes, want %d", f.Name, info.Size(), f.Size)
		}
		got, err := fetch.SHA256File(path)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		if got != f.SHA256 {
			return fmt.Errorf("%s: checksum mismatch: got sha256:%s, want sha256:%s", f.Name, got, f.SHA256)
		}
	}
	return nil
}

// fetchManifest downloads and parses a manifest.json.
func fetchManifest(ctx context.Context, client *http.Client, url string) (Manifest, error) {
	resp, err := httpGet(ctx, client, url)
	if err != nil {
		return Manifest{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var m Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("parsing %s: %w", url, err)
	}
	return m, nil
}

// fetchTarball downloads a zstd-compressed tar archive from url and extracts
// its regular files directly into destDir (flattened: directory entries and
// any path components in tar headers are not preserved beyond the base
// name), rejecting entries whose name would escape destDir.
func fetchTarball(ctx context.Context, client *http.Client, url, destDir string) error {
	resp, err := httpGet(ctx, client, url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	zr, err := zstd.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("creating zstd reader for %s: %w", url, err)
	}
	defer zr.Close()

	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading tar entry from %s: %w", url, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		name := filepath.Clean(hdr.Name)
		if name == "." || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || filepath.IsAbs(name) {
			return fmt.Errorf("tar entry %q in %s has an unsafe path", hdr.Name, url)
		}

		dest := filepath.Join(destDir, name)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(dest), err)
		}
		if err := extractFile(tr, dest); err != nil {
			return err
		}
	}
}

func extractFile(r io.Reader, dest string) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dest, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	return f.Close()
}

func httpGet(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("fetching %s: unexpected status %s", url, resp.Status)
	}
	return resp, nil
}
