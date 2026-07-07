// Package pizerow implements internal/boards.Board for the original
// Raspberry Pi Zero W: GPU boot firmware and config.txt/cmdline.txt in the
// FAT boot partition (no U-Boot - the GPU ROM loads kernel.img directly,
// same as the Pi Zero 2W), and WiFi firmware (plus its board-specific alias
// names) under /lib/firmware in the initramfs. Unlike the Zero 2W, this
// board is 32-bit only (BCM2835, armv6) - see Arch. Pinned sources live in
// build/boards/pi-zero-w/manifest.json; locked template content lives in
// this package's templates sub-package. See bean gosd-et0q (epic
// gosd-ajpz).
package pizerow

import (
	"fmt"
	"io"
	"path"
	"strings"

	manifest "github.com/jphastings/gosd/build/boards/pi-zero-w"
	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/pizerow/templates"
	"github.com/jphastings/gosd/internal/image"
)

const (
	// boardName is the --board flag value and Artifacts() key namespace.
	boardName = "pi-zero-w"

	// kernelArtifactName and dtbArtifactName are the artifacts the
	// pipeline must resolve for the kernel image and device tree blob
	// config.txt names ("kernel=kernel.img") and the GPU ROM loads.
	// Neither has a per-file pinned URL (ArtifactRef.URL is empty):
	// they're compiled by build/boards/pi-zero-w/build.sh and resolved
	// either from --artifacts-dir or, falling back, from the CI-built
	// artifact release (see bean gosd-wtpa and internal/artifacts).
	kernelArtifactName = "kernel.img"
	dtbArtifactName    = "bcm2835-rpi-zero-w.dtb"

	// initramfsName is the file name the initramfs is written under in
	// the FAT boot partition; config.txt's "initramfs" directive and
	// this name must match.
	initramfsName = "initramfs.cpio.zst"
)

type board struct{}

// New returns the pi-zero-w Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// Arch implements boards.Board: the Pi Zero W's BCM2835 has a single
// ARM1176JZF-S core - armv6, 32-bit only, unlike every other GoSD board
// (see bean gosd-ajpz).
func (board) Arch() boards.Arch { return boards.Arch{GOARCH: "arm", GOARM: "6"} }

// Artifacts implements boards.Board: the kernel and DTB (not yet
// automatically fetchable), the GPU boot firmware, and the WiFi firmware
// blobs pinned in manifest.json.
func (board) Artifacts() []boards.ArtifactRef {
	m := manifest.Load()

	refs := make([]boards.ArtifactRef, 0, 2+len(m.BootFiles.Files)+len(m.WifiFirmware.Files))
	refs = append(refs, boards.ArtifactRef{Name: kernelArtifactName}, boards.ArtifactRef{Name: dtbArtifactName})
	refs = append(refs, fileRefs(m.BootFiles.Files)...)
	refs = append(refs, fileRefs(m.WifiFirmware.Files)...)
	return refs
}

func fileRefs(files []manifest.File) []boards.ArtifactRef {
	refs := make([]boards.ArtifactRef, len(files))
	for i, f := range files {
		refs[i] = boards.ArtifactRef{Name: f.Name, URL: f.URL, SHA256: f.SHA256}
	}
	return refs
}

// BootFiles implements boards.Board: the kernel, DTB, GPU boot firmware,
// rendered config.txt/cmdline.txt, and the initramfs the build pipeline has
// already built into art.Initramfs.
func (board) BootFiles(cfg boards.BuildConfig, art boards.Artifacts) (map[string]io.Reader, error) {
	m := manifest.Load()

	files := make(map[string]io.Reader, len(m.BootFiles.Files)+4)

	kernel, err := art.Open(kernelArtifactName)
	if err != nil {
		return nil, err
	}
	files[kernelArtifactName] = kernel

	dtb, err := art.Open(dtbArtifactName)
	if err != nil {
		return nil, err
	}
	files[dtbArtifactName] = dtb

	for _, f := range m.BootFiles.Files {
		r, err := art.Open(f.Name)
		if err != nil {
			return nil, err
		}
		files[f.Name] = r
	}

	configTxt, err := templates.RenderConfigTxt(templates.ConfigTxtData{InitramfsName: initramfsName, UsbGadget: cfg.UsbGadget})
	if err != nil {
		return nil, fmt.Errorf("rendering config.txt: %w", err)
	}
	files["config.txt"] = strings.NewReader(configTxt)

	cmdlineTxt, err := templates.RenderCmdlineTxt(templates.CmdlineTxtData{Board: boardName})
	if err != nil {
		return nil, fmt.Errorf("rendering cmdline.txt: %w", err)
	}
	files["cmdline.txt"] = strings.NewReader(cmdlineTxt)

	if art.Initramfs == nil {
		return nil, fmt.Errorf("pi-zero-w BootFiles: no initramfs archive was provided by the build pipeline")
	}
	files[initramfsName] = art.Initramfs

	return files, nil
}

// RawWrites implements boards.Board: the Pi boots via the GPU ROM and FAT
// partition alone, with no bootloader in the unpartitioned gap.
func (board) RawWrites(boards.Artifacts) []image.RawWrite { return nil }

// FirmwareFiles implements boards.Board: the WiFi firmware blobs, plus the
// board-specific alias names the brcmfmac driver looks up at runtime,
// materialized as duplicate entries (not symlinks - the initramfs format
// doesn't carry those) under brcm/. See manifest.json's wifiFirmware.notes:
// the underlying bytes are Cypress-branded upstream, but every alias is
// flattened into the same brcm/ destDir as the base files here.
func (board) FirmwareFiles(art boards.Artifacts) map[string]io.Reader {
	m := manifest.Load()

	files := make(map[string]io.Reader, len(m.WifiFirmware.Files)+len(m.WifiFirmware.Aliases))
	for _, f := range m.WifiFirmware.Files {
		files[path.Join(m.WifiFirmware.DestDir, f.Name)] = art.MustOpen(f.Name)
	}
	for _, a := range m.WifiFirmware.Aliases {
		files[path.Join(m.WifiFirmware.DestDir, a.Dest)] = art.MustOpen(a.Of)
	}
	return files
}
