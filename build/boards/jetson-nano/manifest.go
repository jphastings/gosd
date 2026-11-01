// Package manifest embeds jetson-nano's pinned third-party artifact manifest
// (manifest.json, in this same directory) so the board profile in
// internal/boards/jetsonnano can consume it without a runtime file read. The
// only blob is the XUSB firmware from linux-firmware: the bootloader lives in
// the module's own QSPI-NOR and GoSD never fetches or ships anything NVIDIA
// built for it (epic gosd-72je's locked decisions).
package manifest

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed manifest.json
var raw []byte

// Manifest is the parsed structure of manifest.json.
type Manifest struct {
	Board    string    `json:"board"`
	Firmware FileGroup `json:"firmware"`
}

// FileGroup is a set of pinned files fetched from the same upstream source,
// landing under DestDir in /lib/firmware.
type FileGroup struct {
	Source  Source `json:"source"`
	DestDir string `json:"destDir"`
	Files   []File `json:"files"`
	Notes   string `json:"notes,omitempty"`
}

// Source records where a FileGroup's files were fetched from, for
// provenance/licensing.
type Source struct {
	Repo   string `json:"repo"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
}

// File pins a single upstream file by URL and expected SHA-256 digest.
type File struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// Load parses the embedded manifest.json. A parse failure here would mean
// the embedded file itself is malformed, which CI's tests would catch
// immediately, so Load treats it as a programmer error rather than a
// runtime one.
func Load() Manifest {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(fmt.Sprintf("manifest.json is invalid: %v", err))
	}
	return m
}
