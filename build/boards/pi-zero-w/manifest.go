// Package manifest embeds pi-zero-w's pinned third-party artifact manifest
// (manifest.json, in this same directory) so the board profile in
// internal/boards/pizerow can consume it without a runtime file read. See
// bean gosd-06kj for the manifest's content and pinning rationale: GPU boot
// firmware from raspberrypi/firmware (same tag as pi-zero-2w), and WiFi
// firmware (plus the board alias names it must be duplicated under) from
// RPi-Distro/firmware-nonfree.
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
	Board        string       `json:"board"`
	BootFiles    FileGroup    `json:"bootFiles"`
	WifiFirmware WifiFirmware `json:"wifiFirmware"`
}

// FileGroup is a set of pinned files fetched from the same upstream source.
type FileGroup struct {
	Source  Source `json:"source"`
	DestDir string `json:"destDir"`
	Files   []File `json:"files"`
}

// Source records where a FileGroup's files were fetched from, for
// provenance/licensing.
type Source struct {
	Repo   string `json:"repo"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
	Path   string `json:"path"`
}

// File pins a single upstream file by URL and expected SHA-256 digest.
type File struct {
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	SHA256        string   `json:"sha256"`
	ChipRevisions []string `json:"chipRevisions,omitempty"`
}

// WifiFirmware is the Zero W's WiFi firmware group: the real blob sets
// (Files) plus the board-specific alias file names (Aliases) the brcmfmac
// driver looks for, each of which must be materialized as a duplicate of
// one of Files. Unlike pi-zero-2w, the underlying blob bytes are Cypress-
// branded (fetched from upstream's cypress/ directory) even though they're
// flattened into the same brcm/ DestDir here - see this package's
// manifest.json "notes" field.
type WifiFirmware struct {
	Source  Source  `json:"source"`
	DestDir string  `json:"destDir"`
	Files   []File  `json:"files"`
	Aliases []Alias `json:"aliases"`
	Notes   string  `json:"notes,omitempty"`
}

// Alias is a board-specific firmware file name that must be materialized as
// a duplicate of the real blob named Of.
type Alias struct {
	Dest string `json:"dest"`
	Of   string `json:"of"`
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
