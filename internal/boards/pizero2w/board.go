// Package pizero2w implements internal/boards.Board for the Raspberry Pi
// Zero 2 W: GPU boot firmware and config.txt/cmdline.txt in the FAT boot
// partition (no U-Boot - the GPU ROM loads kernel8.img directly), and WiFi
// firmware (plus its board-specific alias names) under /lib/firmware in the
// initramfs. Pinned sources live in build/boards/pi-zero-2w/manifest.json;
// locked template content lives in this package's templates sub-package.
// See bean gosd-eu2x.
package pizero2w

import (
	"fmt"
	"io"
	"path"
	"strings"

	manifest "github.com/jphastings/gosd/build/boards/pi-zero-2w"
	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/pizero2w/templates"
	"github.com/jphastings/gosd/internal/image"
)

const (
	// boardName is the --board flag value and Artifacts() key namespace.
	boardName = "pi-zero-2w"

	// kernelArtifactName and dtbArtifactName are the artifacts the
	// pipeline must resolve for the kernel image config.txt names
	// ("kernel=kernel8.img") and the device tree blob the GPU ROM loads.
	// Neither has a per-file pinned URL (ArtifactRef.URL is empty):
	// they're files GoSD compiles itself, resolved either from
	// --artifacts-dir or, falling back, from the CI-built artifact
	// release (see bean gosd-wtpa and internal/artifacts).
	kernelArtifactName = "kernel8.img"
	dtbArtifactName    = "bcm2710-rpi-zero-2-w.dtb"

	// initramfsName is the file name the initramfs is written under in
	// the FAT boot partition; config.txt's "initramfs" directive and
	// this name must match.
	initramfsName = "initramfs.cpio.zst"

	// displayName is this board's human-readable name (see
	// boards.Board.DisplayName), matching COMPATIBILITY.md's prose.
	displayName = "Raspberry Pi Zero 2W"

	// defaultConsoleBaud is this board's own console rate, used whenever
	// BuildConfig.ConsoleBaud is unset (0) - see bean gosd-eu2x's
	// research. --console-baud overrides it; see bean gosd-zp9s.
	defaultConsoleBaud = 115200
)

type board struct{}

// New returns the pi-zero-2w Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// DisplayName implements boards.Board.
func (board) DisplayName() string { return displayName }

// Arch implements boards.Board: the Pi Zero 2W's BCM2837 is 64-bit capable,
// so it runs the same arm64 kernel/userspace as the Radxa Zero 3E (unlike
// its 32-bit-only predecessor, the Pi Zero W - see bean gosd-ajpz).
func (board) Arch() boards.Arch { return boards.Arch{GOARCH: "arm64"} }

// Artifacts implements boards.Board: the kernel and DTB (not yet
// automatically fetchable), the GPU boot firmware, the dwc2 overlay, and the
// WiFi firmware blobs pinned in manifest.json. The overlay is always
// resolved (Artifacts has no build config to consult) but only shipped by
// BootFiles when BuildConfig.UsbGadget is set.
func (board) Artifacts() []boards.ArtifactRef {
	m := manifest.Load()

	refs := make([]boards.ArtifactRef, 0, 2+len(m.BootFiles.Files)+len(m.Overlays.Files)+len(m.WifiFirmware.Files))
	refs = append(refs, boards.ArtifactRef{Name: kernelArtifactName}, boards.ArtifactRef{Name: dtbArtifactName})
	refs = append(refs, fileRefs(m.BootFiles.Files)...)
	refs = append(refs, fileRefs(m.Overlays.Files)...)
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

	// The dwc2 overlay ships only alongside config.txt's conditional
	// "dtoverlay=dwc2" line: without the .dtbo on the boot partition the
	// firmware skips that directive silently, and the gadget package never
	// gets a UDC (bean gosd-spjt).
	if cfg.UsbGadget {
		for _, f := range m.Overlays.Files {
			r, err := art.Open(f.Name)
			if err != nil {
				return nil, err
			}
			files[path.Join(m.Overlays.DestDir, f.Name)] = r
		}
	}

	configTxt, err := templates.RenderConfigTxt(templates.ConfigTxtData{InitramfsName: initramfsName, UsbGadget: cfg.UsbGadget})
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
		return nil, fmt.Errorf("pi-zero-2w BootFiles: no initramfs archive was provided by the build pipeline")
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
// doesn't carry those) under brcm/.
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

// UsbGadgetSupport implements boards.Board: supported. This board's dwc2
// controller is put into peripheral mode by the dtoverlay BootFiles renders
// into config.txt when BuildConfig.UsbGadget is set, giving the gadget
// package a UDC to bind to.
func (board) UsbGadgetSupport() boards.GadgetSupport {
	return boards.GadgetSupport{Supported: true}
}

// ConsoleBaudSupport implements boards.Board: supported. cmdline.txt's
// console= argument carries the baud rate BootFiles renders it with.
func (board) ConsoleBaudSupport() boards.ConsoleBaudSupport {
	return boards.ConsoleBaudSupport{Supported: true}
}

// EXT4Support implements boards.Board: supported. This board's stock kernel
// has built CONFIG_EXT4_FS in since artifacts v0.10.0 (bean gosd-19kw).
func (board) EXT4Support() boards.EXT4Support {
	return boards.EXT4Support{Supported: true}
}
