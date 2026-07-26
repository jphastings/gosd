// Package pi3b implements internal/boards.Board for the Raspberry Pi 3B
// family - one image covers both the 3B and the 3B+ (bean gosd-oq0z): GPU
// boot firmware and config.txt/cmdline.txt in the FAT boot partition
// (no U-Boot - the GPU ROM loads kernel8.img directly, same as the Pi Zero
// 2W), both models' DTBs (the firmware picks by board revision), and WiFi
// firmware (plus its board-specific alias names) under /lib/firmware in the
// initramfs. The BCM2837 is the same arm64 family as the Zero 2W; what sets
// this board apart is onboard wired Ethernet (on the SoC's only USB port: a
// LAN9514 USB hub + 100Mbit chip on the 3B, a LAN7515 GbE chip on the 3B+)
// - and, consequently, no USB gadget support ever (see UsbGadgetSupport).
// Pinned sources live in build/boards/pi-3b/manifest.json; locked template
// content lives in this package's templates sub-package. See bean gosd-ypg1
// (epic gosd-xhc3).
package pi3b

import (
	"fmt"
	"io"
	"path"
	"strings"

	manifest "github.com/jphastings/gosd/build/boards/pi-3b"
	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/pi3b/templates"
	"github.com/jphastings/gosd/internal/image"
)

const (
	// boardName is the --board flag value and Artifacts() key namespace.
	boardName = "pi-3b"

	// kernelArtifactName and the two DTB names are the artifacts the
	// pipeline must resolve for the kernel image config.txt names
	// ("kernel=kernel8.img") and the device tree blobs the GPU ROM picks
	// from by board revision - the 3B's and the 3B+'s ship side by side
	// (the 2026-07-26 maiden boot's 3B+ firmware requested the -plus blob
	// first; bean gosd-oq0z). None has a per-file pinned URL
	// (ArtifactRef.URL is empty): they're compiled by
	// `gosd build-kernel --board pi-3b` and resolved either from
	// --artifacts-dir or, falling back, from the CI-built artifact release
	// (see bean gosd-wtpa and internal/artifacts).
	kernelArtifactName  = "kernel8.img"
	dtbArtifactName     = "bcm2710-rpi-3-b.dtb"
	dtbPlusArtifactName = "bcm2710-rpi-3-b-plus.dtb"

	// initramfsName is the file name the initramfs is written under in
	// the FAT boot partition; config.txt's "initramfs" directive and
	// this name must match.
	initramfsName = "initramfs.cpio.zst"

	// defaultConsoleBaud is this board's own console rate, used whenever
	// BuildConfig.ConsoleBaud is unset (0) - the Pi-standard 115200 on
	// the mini-UART (serial0; BT holds the PL011). --console-baud
	// overrides it; see bean gosd-zp9s.
	defaultConsoleBaud = 115200
)

type board struct{}

// New returns the pi-3b Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// Arch implements boards.Board: the Pi 3B's BCM2837 is the same 64-bit SoC
// family as the Pi Zero 2W, so it runs the same arm64 kernel/userspace.
func (board) Arch() boards.Arch { return boards.Arch{GOARCH: "arm64"} }

// Artifacts implements boards.Board: the kernel and both family DTBs
// (artifact-release resolved), the GPU boot firmware, and the WiFi firmware
// blobs pinned in manifest.json.
func (board) Artifacts() []boards.ArtifactRef {
	m := manifest.Load()

	refs := make([]boards.ArtifactRef, 0, 3+len(m.BootFiles.Files)+len(m.WifiFirmware.Files))
	refs = append(refs,
		boards.ArtifactRef{Name: kernelArtifactName},
		boards.ArtifactRef{Name: dtbArtifactName},
		boards.ArtifactRef{Name: dtbPlusArtifactName},
	)
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

// BootFiles implements boards.Board: the kernel, both family DTBs, GPU boot
// firmware, rendered config.txt/cmdline.txt, and the initramfs the build
// pipeline has already built into art.Initramfs.
func (board) BootFiles(cfg boards.BuildConfig, art boards.Artifacts) (map[string]io.Reader, error) {
	m := manifest.Load()

	files := make(map[string]io.Reader, len(m.BootFiles.Files)+5)

	for _, name := range []string{kernelArtifactName, dtbArtifactName, dtbPlusArtifactName} {
		r, err := art.Open(name)
		if err != nil {
			return nil, err
		}
		files[name] = r
	}

	for _, f := range m.BootFiles.Files {
		r, err := art.Open(f.Name)
		if err != nil {
			return nil, err
		}
		files[f.Name] = r
	}

	configTxt, err := templates.RenderConfigTxt(templates.ConfigTxtData{InitramfsName: initramfsName})
	if err != nil {
		return nil, fmt.Errorf("rendering config.txt: %w", err)
	}
	files["config.txt"] = strings.NewReader(configTxt)

	consoleBaud := cfg.ConsoleBaud
	if consoleBaud == 0 {
		consoleBaud = defaultConsoleBaud
	}
	cmdlineTxt, err := templates.RenderCmdlineTxt(templates.CmdlineTxtData{Board: boardName, ConsoleBaud: consoleBaud})
	if err != nil {
		return nil, fmt.Errorf("rendering cmdline.txt: %w", err)
	}
	files["cmdline.txt"] = strings.NewReader(cmdlineTxt)

	if art.Initramfs == nil {
		return nil, fmt.Errorf("pi-3b BootFiles: no initramfs archive was provided by the build pipeline")
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
// the underlying bytes are the same Cypress-branded 43430 set as pi-zero-w,
// flattened into the same brcm/ destDir under 3-model-b alias names here.
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

// UsbGadgetSupport implements boards.Board: unsupported, permanently. The
// BCM2837's single USB port is hard-wired through the onboard LAN9514 hub
// (which also carries the board's Ethernet), so the controller can never be
// put into peripheral mode - there is no UDC for the gadget package to bind
// to, and no kernel or overlay change can create one. This is a hardware
// property of the 3B, unlike the Pi Zeros whose identical SoC USB is routed
// straight to a port.
func (board) UsbGadgetSupport() boards.GadgetSupport {
	return boards.GadgetSupport{
		Supported: false,
		Reason:    "the Pi 3B's USB is hard-wired through its onboard LAN9514 hub, so the port can never be a USB peripheral - use a Pi Zero 2W or Zero W for gadget mode",
	}
}

// ConsoleBaudSupport implements boards.Board: supported. cmdline.txt's
// console= argument carries the baud rate BootFiles renders it with.
func (board) ConsoleBaudSupport() boards.ConsoleBaudSupport {
	return boards.ConsoleBaudSupport{Supported: true}
}
