// Package fetch downloads pinned upstream files and verifies their content
// against an expected SHA-256 checksum before caching them locally.
//
// It exists because GoSD never re-hosts third-party binary blobs (GPU boot
// firmware, WiFi firmware, bootloader binaries): board manifests pin an
// upstream URL and a SHA-256 digest, and this package is the one place that
// turns that pin into bytes on disk, verified.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// File pins a single downloadable artifact: where to fetch it, and the
// SHA-256 digest (lowercase hex) its content must match.
type File struct {
	URL    string
	SHA256 string
}

// ToDir fetches f into cacheDir/name, verifying its content against
// f.SHA256 before it is made visible under that name.
//
// If cacheDir/name already exists and matches f.SHA256, ToDir returns its
// path without making a network request. Otherwise it downloads f.URL into a
// temporary file in cacheDir, verifies the checksum, and atomically renames
// it into place so concurrent readers never observe a partial file.
//
// A checksum mismatch is reported with both digests so the caller can tell
// whether the upstream file changed or the transfer was corrupted; the
// temporary file is removed and cacheDir/name is left untouched.
//
// ToDir is safe to call repeatedly (e.g. once per board) with a shared
// cacheDir and http.Client; a nil client uses http.DefaultClient.
func ToDir(ctx context.Context, client *http.Client, f File, cacheDir, name string) (string, error) {
	if f.SHA256 == "" {
		return "", fmt.Errorf("fetch %s: no SHA-256 checksum pinned; refusing to download unverified content", f.URL)
	}
	if client == nil {
		client = http.DefaultClient
	}

	dest := filepath.Join(cacheDir, name)
	if _, err := os.Stat(dest); err == nil {
		if got, err := SHA256File(dest); err == nil && got == f.SHA256 {
			return dest, nil
		}
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("creating cache dir %s: %w", cacheDir, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return "", fmt.Errorf("building request for %s: %w", f.URL, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", f.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: unexpected status %s", f.URL, resp.Status)
	}

	tmp, err := os.CreateTemp(cacheDir, name+".part-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file in %s: %w", cacheDir, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once the rename below succeeds

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("downloading %s: %w", f.URL, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("writing %s: %w", tmpPath, err)
	}

	if got := hex.EncodeToString(h.Sum(nil)); got != f.SHA256 {
		return "", fmt.Errorf("%s: checksum mismatch: got sha256:%s, want sha256:%s (upstream file may have changed, or the download was corrupted; re-run to retry, or update the pinned checksum if the change is expected)", f.URL, got, f.SHA256)
	}

	if err := os.Rename(tmpPath, dest); err != nil {
		return "", fmt.Errorf("moving downloaded file into place at %s: %w", dest, err)
	}

	return dest, nil
}

// SHA256File returns the lowercase-hex SHA-256 digest of the file at path.
// It exists (rather than being a private helper of ToDir) so other packages
// that verify pinned files by sha256 — e.g. internal/artifacts, which
// verifies every file inside a downloaded artifact tarball — don't
// reimplement it.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
