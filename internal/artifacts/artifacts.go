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
//
// The manifest is what those per-file digests are read from, so it is
// itself pinned in source — ManifestSHA256, next to Version — and re-checked
// against its bytes every time they are read, downloaded or cached alike.
// Before that anchor existed (bean gosd-1jjh) the cached manifest vouched
// only for itself: anything able to write to the user's cache directory
// could pair a backdoored kernel with a manifest listing that kernel's
// digest, and every later build would take the offline cache-hit branch and
// bake it into every image, without a single network request to notice.
// Tampering with the cache now costs a re-download and nothing more, which
// is exactly how internal/fetch has always treated its own cache.
package artifacts

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
// docs/artifacts.md) is what moves gosd onto newer CI-built kernels/U-Boot —
// always together with ManifestSHA256 below, which is that release's own
// manifest.json.
//
// v0.9.0 carries the first cubie-a5e artifacts (epic gosd-h1wv, bean
// gosd-axtv's kernel build): Image plus sun55i-a527-cubie-a5e.dtb, a
// trimmed mainline v6.18.37 kernel (the fleet's first Allwinner member),
// and u-boot-sunxi-with-spl.bin - a single SPL+FIT bootloader image with
// BL31 compiled from a pinned TF-A fork (mainline has no sun55i_a523
// platform yet), no rkbin-style blobs.
//
// v0.10.0 (bean gosd-toic): the Pi-family kernels gain CONFIG_EXT4_FS=y
// (bean gosd-19kw), unlocking ext4 - including disk's default - on
// attached storage for pi-zero-2w/pi-zero-w/pi-3b, and it is the first
// published build of the radxa-zero-3e/nanopi-zero2 exFAT fragments.
// Other boards are unchanged rebuilds from identical source pins.
//   - v0.10.1: Cubie A5E images now boot the 1GB RAM variant.
//   - v0.10.2: The Cubie A5E kernel build now produces a USB-gadget
//     variant device tree; Cubie A5E U-Boot no longer scans USB on every
//     boot.
//   - v0.10.3: The status LED's fatal signal now survives kernel
//     shutdown; SPI now works on the Raspberry Pi Zero W.
//   - v0.10.4: Turing RK1 kernel and U-Boot are now published in
//     artifacts releases.
const Version = "v0.10.4"

// ManifestSHA256 is the SHA-256 (lowercase hex) of the manifest.json
// published with the artifacts/<Version> release. It is the trust anchor for
// everything this package reads: every per-file digest EnsureBoard verifies
// against comes out of that manifest, so the manifest's own bytes are
// checked against this compiled-in value before any entry inside it is
// believed — whether they arrived over the network or out of the user's
// cache directory (bean gosd-1jjh).
//
// It moves with Version, never separately. build/artifacts/pin-bump.sh
// rewrites both (see docs/artifacts.md); by hand, it is
//
//	curl -sfL https://github.com/jphastings/gosd/releases/download/artifacts/<Version>/manifest.json | shasum -a 256
const ManifestSHA256 = "6a74768859176c3ead4818e3380f58543b808dd01c9b0fee82f31fd28d6e18d2"

// repoSlug is the GitHub repository artifact releases are published to.
const repoSlug = "jphastings/gosd"

// maxUnpackedBytes caps how much one board tarball may decompress to before
// extraction is abandoned. Every board's artifacts together are tens of MiB
// today, so this is more than an order of magnitude of headroom; its only
// job is to stop a corrupt or hostile archive from filling the disk, since
// extraction necessarily happens before verifyFiles can reject what came
// out of it.
const maxUnpackedBytes = 1 << 30 // 1 GiB

// maxManifestBytes bounds the manifest read: a few tens of KiB describe
// every board, and the digest check can only run on bytes already in
// memory, so an endless response must not be a way to get there.
const maxManifestBytes = 4 << 20 // 4 MiB

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
// A nil client uses fetch.DefaultClient. Later calls with the same cacheDir
// make no network request at all, provided the cached files still verify
// against a cached manifest that itself still matches ManifestSHA256 —
// gosd build works fully offline after the first successful call.
func EnsureBoard(ctx context.Context, client *http.Client, cacheDir, board string) (string, error) {
	return ensureBoard(ctx, client, cacheDir, board, Version, ManifestSHA256, func(name string) string {
		return fmt.Sprintf("https://github.com/%s/releases/download/artifacts/%s/%s", repoSlug, Version, name)
	})
}

// ensureBoard is EnsureBoard's testable core: urlFor maps a release asset
// name ("manifest.json" or "<board>.tar.zst") to a download URL, so tests
// can point it at an httptest.Server instead of GitHub.
func ensureBoard(ctx context.Context, client *http.Client, cacheDir, board, version, manifestSHA256 string, urlFor func(name string) string) (string, error) {
	if board == "" {
		return "", errors.New("artifacts: board name is required")
	}
	if manifestSHA256 == "" {
		return "", fmt.Errorf(
			"this build of gosd pins artifacts release %s but no checksum for its manifest.json, so it can't tell a genuine release from a tampered one and won't use either; "+
				"this is a bug in the build — internal/artifacts.ManifestSHA256 must be set alongside Version (see docs/artifacts.md) — so report it, or build for now with --artifacts-dir",
			version)
	}
	if client == nil {
		client = fetch.DefaultClient
	}

	versionDir := filepath.Join(cacheDir, version)
	boardDir := filepath.Join(versionDir, board)
	manifestPath := filepath.Join(versionDir, "manifest.json")

	if m, err := readManifestCache(manifestPath, manifestSHA256); err == nil {
		if bf, ok := m.Boards[board]; ok && verifyFiles(boardDir, bf.Files) == nil {
			return boardDir, nil // fully offline: cache already verified, no network touched
		}
	}

	manifestBytes, manifest, err := fetchManifest(ctx, client, urlFor("manifest.json"), manifestSHA256)
	if errors.Is(err, errUntrustedManifest) {
		return "", err // says what went wrong on its own terms; being offline is not one of the explanations
	}
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

	if err := fetchTarball(ctx, client, urlFor(board+".tar.zst"), tmpDir, maxUnpackedBytes); err != nil {
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

	if err := writeManifestCache(manifestPath, manifestBytes); err != nil {
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

// readManifestCache reads a previously-cached manifest.json, re-hashing its
// bytes against the pinned digest before parsing them — the cached file is
// as untrusted as a downloaded one, so a mismatch is reported as an error
// and ensureBoard falls through to a fresh download.
func readManifestCache(path, wantSHA256 string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	return parseManifest(data, wantSHA256, path)
}

// parseManifest verifies data against wantSHA256 — described to the reader
// as source — and only then unmarshals it.
func parseManifest(data []byte, wantSHA256, source string) (Manifest, error) {
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != wantSHA256 {
		return Manifest{}, fmt.Errorf("%s: checksum mismatch: got sha256:%s, want sha256:%s", source, got, wantSHA256)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing manifest %s: %w", source, err)
	}
	return m, nil
}

// writeManifestCache persists the verified manifest bytes — byte for byte as
// published, so a later readManifestCache can re-check them against the same
// pinned digest — so future EnsureBoard calls (for this or any other board
// at the same version) can verify their cache without a network request.
func writeManifestCache(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "manifest.json.part-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", filepath.Dir(path), err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", tmpPath, err)
	}
	return os.Rename(tmpPath, path)
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

// errUntrustedManifest marks a manifest that downloaded fine but isn't the
// one this gosd was built against — a different failure from not reaching
// the release at all, and one no amount of network connectivity fixes.
var errUntrustedManifest = errors.New("the artifact manifest is not the one this gosd was built against")

// fetchManifest downloads a manifest.json, verifies its bytes against
// wantSHA256, and returns both those bytes and the parsed result — the bytes
// so the caller can cache exactly what it verified.
func fetchManifest(ctx context.Context, client *http.Client, url, wantSHA256 string) ([]byte, Manifest, error) {
	resp, err := httpGet(ctx, client, url)
	if err != nil {
		return nil, Manifest{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
	if err != nil {
		return nil, Manifest{}, fmt.Errorf("reading %s: %w", url, err)
	}

	m, err := parseManifest(data, wantSHA256, url)
	if err != nil {
		return nil, Manifest{}, fmt.Errorf(
			"%w: %w; this gosd only trusts the artifact release it was built against, so nothing has been "+
				"downloaded or cached — re-run in case the transfer was corrupted, and if it keeps failing "+
				"report it, or build from artifacts you supply yourself with --artifacts-dir",
			errUntrustedManifest, err)
	}
	return data, m, nil
}

// fetchTarball downloads a zstd-compressed tar archive from url and extracts
// its regular files directly into destDir (flattened: directory entries and
// any path components in tar headers are not preserved beyond the base
// name), rejecting entries whose name would escape destDir and refusing to
// write more than maxUnpacked bytes in total.
func fetchTarball(ctx context.Context, client *http.Client, url, destDir string, maxUnpacked int64) error {
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

	tr := tar.NewReader(&cappedReader{
		r:         zr,
		remaining: maxUnpacked,
		err: fmt.Errorf(
			"%s expands to more than %d bytes, far larger than any board's artifacts; extraction was abandoned rather than filling the disk",
			url, maxUnpacked),
	})
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

// cappedReader is io.LimitReader that reports its own error on running out
// rather than a clean EOF, so an over-long archive fails as what it is
// instead of as a truncated tar.
type cappedReader struct {
	r         io.Reader
	remaining int64
	err       error
}

func (c *cappedReader) Read(p []byte) (int, error) {
	if c.remaining <= 0 {
		return 0, c.err
	}
	if int64(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.r.Read(p)
	c.remaining -= int64(n)
	return n, err
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
