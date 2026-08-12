// Package inject implements the gosd side of the image-injection contract:
// deterministic, comment-padded placeholder files pre-created on the FAT32
// boot partition at build time (`gosd build --placeholder
// <path>=<size>`), and the <image basename>.inject.json manifest that
// records the absolute byte ranges each placeholder's content occupies in
// the finished .img. A provisioning tool downloads the image, verifies
// hashes against the manifest, and overwrites those ranges with
// same-length bytes via a plain os.WriteAt - no FAT tooling required - and
// the FAT-level file reads back patched. See docs/image-injection.md for
// the full contract.
package inject

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jphastings/gosd/internal/image"
)

// Placeholder describes one --placeholder <path>=<size> flag: a file gosd
// build reserves at Path on the FAT root of the boot partition, rendered
// deterministically to exactly SizeBytes bytes (see Render).
type Placeholder struct {
	// Path is the placeholder's location on the FAT root: forward-slash
	// separated, relative, with no leading slash and no "." or ".."
	// segments.
	Path string
	// SizeBytes is the placeholder's exact rendered size.
	SizeBytes int64
}

// placeholderHeaderLine1Prefix is the fixed prefix of Render's first line;
// Render appends "<path>\n" to it.
const placeholderHeaderLine1Prefix = "# GOSD-PLACEHOLDER v1 path="

// placeholderExplanation is the fixed explanatory comment block Render
// writes right after the first line, verbatim - see docs/image-injection.md
// for the contract it documents.
const placeholderExplanation = `# This file is reserved space: a provisioning tool fills it in place at
# image-download time, and you may also simply replace it with your own
# content. Programs treat a file still beginning with the line above as
# absent.
`

// paddingLineBytes is one full padding line: 79 '#' characters followed by
// a newline (80 bytes) - Render repeats this while more than 80 bytes of
// padding remain.
var paddingLineBytes = append(bytes.Repeat([]byte{'#'}, 79), '\n')

// pathSegmentPattern is the shape each forward-slash-separated component of
// a placeholder Path must match. The path is embedded verbatim in the
// rendered file's first line and in the manifest's JSON, so keeping it
// boring (no spaces, no shell/YAML/JSON metacharacters) avoids reasoning
// about escaping in either place.
var pathSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// maxPlaceholderSizeBytes is FAT32's per-file size ceiling: 4GiB-1, the
// largest value its 32-bit directory-entry file-size field can hold.
const maxPlaceholderSizeBytes = 4*1024*1024*1024 - 1

// minRenderedSizeBytes returns the smallest SizeBytes Render can produce
// for path: the deterministic header (first line + explanation) with no
// padding bytes to spare.
func minRenderedSizeBytes(path string) int64 {
	return int64(len(placeholderHeaderLine1Prefix)+len(path)+1) + int64(len(placeholderExplanation))
}

// Validate checks p's Path and SizeBytes are usable by --placeholder: Path
// must be a boring, forward-slash relative path with no leading slash and
// no "." or ".." segments, and SizeBytes must be large enough to hold
// Render's deterministic header and no larger than FAT32's 4GiB-1 per-file
// ceiling.
func (p Placeholder) Validate() error {
	if p.Path == "" {
		return errors.New("--placeholder path must not be empty")
	}
	if strings.HasPrefix(p.Path, "/") {
		return fmt.Errorf("--placeholder path %q must not start with '/'; it's relative to the FAT root of the boot partition", p.Path)
	}
	for _, seg := range strings.Split(p.Path, "/") {
		if seg == "." || seg == ".." {
			return fmt.Errorf("--placeholder path %q must not contain '.' or '..' segments", p.Path)
		}
		if !pathSegmentPattern.MatchString(seg) {
			return fmt.Errorf("--placeholder path %q has an invalid path segment %q; each segment must match %s", p.Path, seg, pathSegmentPattern.String())
		}
	}

	if min := minRenderedSizeBytes(p.Path); p.SizeBytes < min {
		return fmt.Errorf("--placeholder %s=%d is too small; the minimum size for this path is %d bytes (Render's fixed header)", p.Path, p.SizeBytes, min)
	}
	if p.SizeBytes > maxPlaceholderSizeBytes {
		return fmt.Errorf("--placeholder %s=%d exceeds FAT32's per-file limit of %d bytes (4GiB-1)", p.Path, p.SizeBytes, maxPlaceholderSizeBytes)
	}
	return nil
}

// Render deterministically renders p into exactly p.SizeBytes bytes: a
// first line naming p.Path, a short human-readable explanation, then
// '#'-comment padding to the exact size (final byte '\n'). The result is
// valid YAML (comments and blank lines only) and legible to anyone who
// mounts the card - see docs/image-injection.md for the contract this
// implements. Render calls p.Validate() itself.
func Render(p Placeholder) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	buf := make([]byte, 0, p.SizeBytes)
	buf = append(buf, placeholderHeaderLine1Prefix...)
	buf = append(buf, p.Path...)
	buf = append(buf, '\n')
	buf = append(buf, placeholderExplanation...)

	remaining := p.SizeBytes - int64(len(buf))
	for remaining > 80 {
		buf = append(buf, paddingLineBytes...)
		remaining -= 80
	}
	if remaining > 0 {
		buf = append(buf, bytes.Repeat([]byte{'#'}, int(remaining-1))...)
		buf = append(buf, '\n')
	}

	return buf, nil
}

// manifestSchemaVersion is the current value of Manifest.GosdInject.
const manifestSchemaVersion = 1

// Manifest is the JSON schema written to <image>.inject.json by
// WriteManifest - see docs/image-injection.md for a worked example and the
// client algorithm that consumes it.
type Manifest struct {
	GosdInject   int               `json:"gosd_inject"`
	Board        string            `json:"board"`
	Image        ImageInfo         `json:"image"`
	Placeholders []PlaceholderInfo `json:"placeholders"`

	// Env is the reserved [env] region inside gosd.toml
	// (gosd build --env-placeholder), absent when the build reserved none.
	// It is deliberately NOT an entry in Placeholders: those name a real
	// FAT-root file and report its whole content, while this is a span of a
	// file whose remaining bytes must not be touched. The schema version
	// stays 1 - a client that predates this key ignores it and keeps
	// working on the placeholders it does understand.
	Env *EnvInfo `json:"env,omitempty"`
}

// EnvInfo is the reserved [env] region's manifest entry: how many bytes were
// reserved, the SHA-256 of the region as gosd built it, and the ordered
// absolute byte ranges it occupies in the image.
//
// A client overwrites those ranges with exactly Size bytes of TOML [env]
// body - KEY = "value" lines and comments, no section headers, since a
// header would capture every line gosd wrote after the region - padded to
// length with newlines. What lands there is an ordinary gosd.toml value: the
// device merges it with no special case, and the provisioning snapshot
// carries it across a later reflash (docs/image-injection.md).
type EnvInfo struct {
	Size   int64   `json:"size"`
	SHA256 string  `json:"sha256"`
	Ranges []Range `json:"ranges"`
}

// ImageInfo describes the whole pristine image file a Manifest belongs to.
type ImageInfo struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

// PlaceholderInfo is one placeholder's manifest entry: its size, the
// SHA-256 of its rendered content, and the ordered absolute byte ranges
// that content occupies in the image.
type PlaceholderInfo struct {
	Path   string  `json:"path"`
	Size   int64   `json:"size"`
	SHA256 string  `json:"sha256"`
	Ranges []Range `json:"ranges"`
}

// Range is one contiguous, absolute byte range within the image file.
type Range struct {
	Offset int64 `json:"offset"`
	Length int64 `json:"length"`
}

// ManifestPath returns the injection manifest path for an image at imgPath:
// its extension (normally ".img") replaced with ".inject.json" - the same
// sidecar convention as internal/catalog's fragmentPath.
func ManifestPath(imgPath string) string {
	return strings.TrimSuffix(imgPath, filepath.Ext(imgPath)) + ".inject.json"
}

// ManifestSpec is everything WriteManifest needs about a finished build
// besides the image file itself.
type ManifestSpec struct {
	// Board is the board ID the image was built for.
	Board string

	// Placeholders are the --placeholder files this build reserved.
	Placeholders []Placeholder

	// EnvReservedBytes is --env-placeholder's size, or zero when the build
	// reserved no [env] region.
	EnvReservedBytes int64

	// FileRanges is image.WriteReport.FileRanges: the reported byte ranges,
	// keyed by boot-file path. It must hold an entry for every placeholder's
	// Path, and for "gosd.toml" when EnvReservedBytes is non-zero.
	FileRanges map[string][]image.ByteRange
}

// WriteManifest streams imgPath (the just-built, pristine image) through
// SHA-256, builds a Manifest recording each placeholder's rendered content
// hash and reported byte ranges (plus the reserved [env] region, when there
// is one), and writes it to ManifestPath(imgPath) as indented JSON. It
// returns the path written.
//
// spec.FileRanges must have an entry for every placeholder's Path -
// internal/image.Spec.ReportRanges guarantees this for a build that
// included every placeholder's path - and each entry's ranges must sum to
// exactly its placeholder's SizeBytes (image.Write's own clipping
// guarantees this too; the check here is defensive).
func WriteManifest(imgPath string, spec ManifestSpec) (string, error) {
	size, sha256Hex, err := hashAndSize(imgPath)
	if err != nil {
		return "", fmt.Errorf("hashing image %s for its injection manifest failed: %w", imgPath, err)
	}

	env, err := envInfo(imgPath, spec)
	if err != nil {
		return "", err
	}

	placeholders, fileRanges := spec.Placeholders, spec.FileRanges
	infos := make([]PlaceholderInfo, 0, len(placeholders))
	for _, p := range placeholders {
		rendered, err := Render(p)
		if err != nil {
			return "", fmt.Errorf("re-rendering placeholder %q for its manifest hash failed: %w", p.Path, err)
		}
		sum := sha256.Sum256(rendered)

		ranges, ok := fileRanges[p.Path]
		if !ok {
			return "", fmt.Errorf("no reported byte ranges for placeholder %q; internal/image.Spec.ReportRanges must include every placeholder path", p.Path)
		}
		manifestRanges := make([]Range, len(ranges))
		var total int64
		for i, r := range ranges {
			manifestRanges[i] = Range{Offset: r.OffsetBytes, Length: r.LengthBytes}
			total += r.LengthBytes
		}
		if total != p.SizeBytes {
			return "", fmt.Errorf("placeholder %q's reported byte ranges total %d bytes, want exactly its size %d", p.Path, total, p.SizeBytes)
		}

		infos = append(infos, PlaceholderInfo{
			Path:   p.Path,
			Size:   p.SizeBytes,
			SHA256: hex.EncodeToString(sum[:]),
			Ranges: manifestRanges,
		})
	}

	manifest := Manifest{
		GosdInject: manifestSchemaVersion,
		Board:      spec.Board,
		Image: ImageInfo{
			Filename: filepath.Base(imgPath),
			Size:     size,
			SHA256:   sha256Hex,
		},
		Placeholders: infos,
		Env:          env,
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding injection manifest for %s failed: %w", imgPath, err)
	}
	data = append(data, '\n')

	path := ManifestPath(imgPath)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("writing injection manifest %s failed: %w", path, err)
	}
	return path, nil
}

// envInfo builds the manifest's reserved-[env] entry, or nil when the build
// reserved none. Unlike a placeholder - which gosd can re-render from its
// spec alone - the region's pristine content is whatever gosd.toml's [env]
// body came out as, so its hash is taken from the bytes actually written to
// the image at the ranges being published. That makes the published hash
// wrong exactly when the published ranges are wrong, which is the failure a
// client's pristine-check exists to catch.
func envInfo(imgPath string, spec ManifestSpec) (*EnvInfo, error) {
	if spec.EnvReservedBytes == 0 {
		return nil, nil
	}

	ranges, ok := spec.FileRanges["gosd.toml"]
	if !ok {
		return nil, fmt.Errorf("no reported byte ranges for gosd.toml's reserved [env] region; internal/image.Spec.ReportRanges must include it whenever --env-placeholder reserves one")
	}

	manifestRanges := make([]Range, len(ranges))
	var total int64
	for i, r := range ranges {
		manifestRanges[i] = Range{Offset: r.OffsetBytes, Length: r.LengthBytes}
		total += r.LengthBytes
	}
	if total != spec.EnvReservedBytes {
		return nil, fmt.Errorf("gosd.toml's reported [env] ranges total %d bytes, want exactly the %d reserved", total, spec.EnvReservedBytes)
	}

	sum, err := hashRanges(imgPath, ranges)
	if err != nil {
		return nil, fmt.Errorf("hashing the reserved [env] region of %s failed: %w", imgPath, err)
	}
	return &EnvInfo{Size: spec.EnvReservedBytes, SHA256: sum, Ranges: manifestRanges}, nil
}

// hashRanges returns the hex-encoded SHA-256 of the image's bytes at ranges,
// concatenated in order - the same bytes, in the same order, a client
// reassembles when it checks the region is still pristine.
func hashRanges(imgPath string, ranges []image.ByteRange) (string, error) {
	f, err := os.Open(imgPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	for _, r := range ranges {
		if _, err := io.Copy(h, io.NewSectionReader(f, r.OffsetBytes, r.LengthBytes)); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashAndSize returns the size in bytes and hex-encoded SHA-256 of the file
// at path (mirrors internal/catalog's helper of the same name/shape).
func hashAndSize(path string) (size int64, sha256Hex string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}
