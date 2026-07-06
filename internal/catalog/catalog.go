// Package catalog builds Raspberry Pi Imager custom-repository catalog
// files (os_list.json) from gosd's own built images, so end users can paste
// a repo URL into Imager's Settings -> Custom repository and get the full
// WiFi/hostname customization wizard instead of the no-customization "Use
// custom image" file picker (see the "End-user flashing path" locked
// decision in CLAUDE.md and the §0 finding in docs/provisioning-formats.md
// that explains why that file picker can't be used instead).
//
// Every entry gosd emits declares init_format "cloudinit": gosd-init only
// ever parses cloud-init files and gosd.toml (never firstrun.sh - see
// docs/provisioning-formats.md), so "cloudinit" is the only format that
// makes sense to advertise.
//
// The generated shape is checked in tests against a pinned copy of
// rpi-imager's own JSON Schema (testdata/os-list-schema.json); see that
// file's sibling README.md for the pinned commit and catalog_test.go for
// why a full JSON-Schema validator dependency wasn't added just for this.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// InitFormatCloudInit is the only init_format gosd ever emits (see the
// package doc comment).
const InitFormatCloudInit = "cloudinit"

// dateFormat is the YYYY-MM-DD layout the schema's release_date field
// requires (see os-list-schema.json's release_date examples).
const dateFormat = "2006-01-02"

// boardDisplayNames maps a board's stable --board ID (Board.Name()) to the
// human-friendly name shown in Imager's "CHOOSE OS" list. A board with no
// entry here (e.g. one registered after this list was last updated) falls
// back to its raw ID, which is legible but less polished - see
// displayName.
var boardDisplayNames = map[string]string{
	"pi-zero-2w":    "Raspberry Pi Zero 2 W",
	"radxa-zero-3e": "Radxa Zero 3E",
}

// displayName returns the human-friendly name for boardID, falling back to
// boardID itself when it isn't in boardDisplayNames.
func displayName(boardID string) string {
	if name, ok := boardDisplayNames[boardID]; ok {
		return name
	}
	return boardID
}

// boardImagerDeviceTags maps a board's --board ID to the official Raspberry
// Pi Imager device tags its images should carry in the entry's "devices"
// array. Imager's device-selection page filters the OS list by intersecting
// each entry's devices with the selected device's tags (imagewriter.cpp's
// filterOsListWithHWTags, rpi-imager v2.0.10 commit 467be3d3), so a tag
// Imager doesn't define hides the entry behind "No filtering".
//
// pi-zero-2w carries "pi3-64bit": in the official catalog
// (downloads.raspberrypi.org/os_list_imagingutility_v4.json) the
// "Raspberry Pi Zero 2 W" device is defined with tags
// ["pi3-64bit", "pi3-32bit"] - it shares the Pi 3's tags; no Zero-2W-only
// tag exists - and GoSD images are arm64-only, so only the 64-bit tag
// applies (the same single tag Zero-2W-capable arm64 images like Home
// Assistant OS use). Side effect of the shared namespace: the entry also
// appears when "Raspberry Pi 3" is selected.
//
// Boards absent from this map (all non-Raspberry-Pi hardware - Imager's
// device list only contains Raspberry Pi models, so no official tag can
// ever match them) fall back to their raw board ID: a deliberately
// non-matching but self-describing tag that satisfies the schema's
// required, conventionally non-empty devices field. Those entries appear
// only under "No filtering" - an Imager limitation documented for
// developers in docs/publishing.md.
var boardImagerDeviceTags = map[string][]string{
	"pi-zero-2w": {"pi3-64bit"},
}

// imagerDeviceTags returns the devices array for boardID's catalog entry -
// official Imager tags where they exist, the raw board ID otherwise (see
// boardImagerDeviceTags for why).
func imagerDeviceTags(boardID string) []string {
	if tags, ok := boardImagerDeviceTags[boardID]; ok {
		return tags
	}
	return []string{boardID}
}

// Entry is one Raspberry Pi Imager os_list.json OS entry: the fields gosd
// populates from a real built image. Field names/JSON tags match
// testdata/os-list-schema.json's "Operating system entry" variant exactly.
type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	URL         string `json:"url"`

	// ExtractSize and ExtractSHA256 describe the UNCOMPRESSED image, per
	// rpi-imager's schema semantics (doc/schema-notes.md's "wrong SHA256
	// hash" pitfall: never hash the compressed download). gosd currently
	// distributes raw, uncompressed .img files, so these are identical to
	// ImageDownloadSize/the download itself - see BuildEntry.
	ExtractSize   int64  `json:"extract_size"`
	ExtractSHA256 string `json:"extract_sha256"`

	// ImageDownloadSize is the size of the file actually downloaded from
	// URL. Kept as its own field (rather than reusing ExtractSize) so a
	// future compressed-distribution mode only has to compute this
	// differently, without changing Entry's shape.
	ImageDownloadSize int64 `json:"image_download_size"`

	ReleaseDate string   `json:"release_date"`
	Devices     []string `json:"devices"`
	InitFormat  string   `json:"init_format"`
}

// List is a full os_list.json document, or one of the per-image fragments
// WriteFiles also writes. It intentionally never sets the schema's optional
// top-level "imager" key: that key is metadata for the Imager application
// itself and, per doc/schema-notes.md's "top-level imager metadata in
// sublists" pitfall, must not appear in anything other than a developer's
// own top-level file - gosd has no opinion on that, so it emits none.
type List struct {
	OSList []Entry `json:"os_list"`
}

// Image is one board's finished build: enough information to compute its
// catalog entry. Path must be the local, uncompressed .img file gosd build
// just wrote to disk.
type Image struct {
	// AppName is the sanitized app name (internal/naming.Sanitize), used
	// in the entry's human-friendly name/description.
	AppName string
	// BoardID is the board's Name() (e.g. "pi-zero-2w").
	BoardID string
	// Path is the local .img file on disk.
	Path string
}

// Options configures catalog generation shared across every image in one
// gosd build invocation.
type Options struct {
	// BaseURL is where the developer will host the .img files (and this
	// catalog); it's joined with each image's filename to build the
	// entry's download url. Required: see JoinURL and cmd/gosd's
	// --publish-base-url flag, which rejects an empty value before any
	// building happens.
	BaseURL string

	// ReleaseDate stamps every generated entry's release_date. Defaults to
	// time.Now().UTC() when zero; overridable so callers (and tests) can
	// pin a reproducible value.
	ReleaseDate time.Time
}

// JoinURL joins baseURL and filename into a download URL, tolerating (and
// normalizing away) any number of trailing slashes on baseURL so
// "https://example.com/x" and "https://example.com/x/" behave identically.
func JoinURL(baseURL, filename string) string {
	return strings.TrimRight(baseURL, "/") + "/" + filename
}

// BuildEntry computes one os_list.json entry for img, reading it from disk
// to compute the uncompressed size/hash (see Entry's ExtractSize/
// ExtractSHA256 doc comments). Returns an error if opts.BaseURL is empty
// (catalog generation always requires a base URL) or if img.Path can't be
// read.
func BuildEntry(img Image, opts Options) (Entry, error) {
	if opts.BaseURL == "" {
		return Entry{}, fmt.Errorf("building a catalog entry for %s requires a base URL to construct its download link; pass --publish-base-url=<https://...>", img.Path)
	}

	size, sum, err := hashAndSize(img.Path)
	if err != nil {
		return Entry{}, fmt.Errorf("reading %s to build its catalog entry failed: %w", img.Path, err)
	}

	releaseDate := opts.ReleaseDate
	if releaseDate.IsZero() {
		releaseDate = time.Now().UTC()
	}

	display := displayName(img.BoardID)
	filename := filepath.Base(img.Path)

	return Entry{
		Name:              fmt.Sprintf("%s (%s)", img.AppName, display),
		Description:       fmt.Sprintf("%s, built with GoSD for %s", img.AppName, display),
		Icon:              "",
		URL:               JoinURL(opts.BaseURL, filename),
		ExtractSize:       size,
		ExtractSHA256:     sum,
		ImageDownloadSize: size,
		ReleaseDate:       releaseDate.Format(dateFormat),
		Devices:           imagerDeviceTags(img.BoardID),
		InitFormat:        InitFormatCloudInit,
	}, nil
}

// WriteFiles builds one catalog entry per image and writes it to disk two
// ways: a per-image fragment file (the image's filename with ".img"
// replaced by ".os_list.json", written next to the image itself) and a
// single combined "os_list.json" in dir containing every entry, sorted by
// board ID for reproducible output. It returns the entries in that same
// order.
func WriteFiles(dir string, images []Image, opts Options) ([]Entry, error) {
	sorted := make([]Image, len(images))
	copy(sorted, images)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].BoardID < sorted[j].BoardID })

	entries := make([]Entry, 0, len(sorted))
	for _, img := range sorted {
		entry, err := BuildEntry(img, opts)
		if err != nil {
			return nil, fmt.Errorf("building catalog entry for board %s: %w", img.BoardID, err)
		}
		entries = append(entries, entry)

		if err := writeList(fragmentPath(img.Path), List{OSList: []Entry{entry}}); err != nil {
			return nil, err
		}
	}

	if err := writeList(filepath.Join(dir, "os_list.json"), List{OSList: entries}); err != nil {
		return nil, err
	}

	return entries, nil
}

// fragmentPath returns the per-image catalog fragment path for an image at
// imgPath: its extension (normally ".img") replaced with ".os_list.json".
func fragmentPath(imgPath string) string {
	ext := filepath.Ext(imgPath)
	return strings.TrimSuffix(imgPath, ext) + ".os_list.json"
}

// writeList marshals list as indented JSON and writes it to path.
func writeList(path string, list List) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding catalog JSON for %s failed: %w", path, err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing catalog file %s failed: %w", path, err)
	}
	return nil
}

// hashAndSize returns the size in bytes and hex-encoded SHA256 of the file
// at path.
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
