// Package picm4 implements internal/boards.Board for the Raspberry Pi
// Compute Module 4 (Lite, no-wireless variant): GPU boot firmware and
// config.txt/cmdline.txt in the FAT boot partition (no U-Boot - the GPU ROM
// loads kernel8.img directly, same as pi-zero-2w and pi-3b), a single DTB
// (the official CM4 IO Board's - the closest available match for any
// third-party CM4 carrier, including this board's target hardware, a Turing
// Pi 2 node slot), and no WiFi firmware at all - this module has no
// wireless hardware, the first Pi board GoSD ships with none.
//
// The BCM2711 is the same arm64 SoC family pi-3b already builds for, but
// with genuinely different peripherals: native GENET Ethernet rather than a
// USB-hosted chip, and the BCM2711 iProc SDHCI storage controller rather
// than BCM2835's sdhost - see build/boards/pi-cm4/kernel.fragment's header
// for the full list of deliberate differences.
//
// USB gadget mode is left unresolved by design (epic gosd-7676's "?"
// decision): the dwc2 controller is compiled into the kernel, but whether
// Turing Pi 2's carrier routes it anywhere accessible is uncharacterized,
// not proven either way - see UsbGadgetSupport.
//
// Pinned sources live in build/boards/pi-cm4/manifest.json; locked template
// content lives in this package's templates sub-package. See bean gosd-1tk8
// (epic gosd-7676).
package picm4

import (
	"fmt"
	"io"
	"strings"

	manifest "github.com/jphastings/gosd/build/boards/pi-cm4"
	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/picm4/templates"
	"github.com/jphastings/gosd/internal/image"
)

const (
	// boardName is the --board flag value and Artifacts() key namespace.
	boardName = "pi-cm4"

	// kernelArtifactName and dtbArtifactName are the artifacts the
	// pipeline must resolve for config.txt's "kernel=kernel8.img" name
	// and the device tree blob the GPU ROM loads. Neither has a
	// per-file pinned URL (ArtifactRef.URL is empty): they're compiled
	// by `gosd build-kernel --board pi-cm4` and resolved either from
	// --artifacts-dir or, falling back, from the CI-built artifact
	// release (see bean gosd-wtpa and internal/artifacts).
	kernelArtifactName = "kernel8.img"
	dtbArtifactName    = "bcm2711-rpi-cm4.dtb"

	// initramfsName is the file name the initramfs is written under in
	// the FAT boot partition; config.txt's "initramfs" directive and
	// this name must match.
	initramfsName = "initramfs.cpio.zst"

	// displayName is this board's human-readable name (see
	// boards.Board.DisplayName), matching COMPATIBILITY.md's prose once
	// this board is activated.
	displayName = "Raspberry Pi CM4"

	// defaultConsoleBaud is this board's own console rate, used whenever
	// BuildConfig.ConsoleBaud is unset (0) - the Pi-standard 115200.
	// --console-baud overrides it; see bean gosd-zp9s.
	defaultConsoleBaud = 115200
)

type board struct{}

// New returns the pi-cm4 Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// DisplayName implements boards.Board.
func (board) DisplayName() string { return displayName }

// Arch implements boards.Board: the BCM2711 is the same 64-bit SoC family
// as the Pi 3B and Pi Zero 2W, so it runs the same arm64 kernel/userspace.
func (board) Arch() boards.Arch { return boards.Arch{GOARCH: "arm64"} }

// Artifacts implements boards.Board: the kernel and DTB (artifact-release
// resolved) plus the GPU boot firmware pinned in manifest.json.
func (board) Artifacts() []boards.ArtifactRef {
	m := manifest.Load()

	refs := make([]boards.ArtifactRef, 0, 2+len(m.BootFiles.Files))
	refs = append(refs,
		boards.ArtifactRef{Name: kernelArtifactName},
		boards.ArtifactRef{Name: dtbArtifactName},
	)
	refs = append(refs, fileRefs(m.BootFiles.Files)...)
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
//
// cfg.UsbGadget is never true here: UsbGadgetSupport reports unsupported,
// so gosd build --usb-gadget refuses for this board before BootFiles is
// ever called with it set (see cmd/gosd's validateUsbGadget). There is
// accordingly no dwc2-overlay branch in config.txt, even though the
// kernel's dwc2 driver is compiled in - see the package doc.
func (board) BootFiles(cfg boards.BuildConfig, art boards.Artifacts) (map[string]io.Reader, error) {
	m := manifest.Load()

	files := make(map[string]io.Reader, len(m.BootFiles.Files)+4)

	for _, name := range []string{kernelArtifactName, dtbArtifactName} {
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
	cmdlineTxt, err := templates.RenderCmdlineTxt(templates.CmdlineTxtData{Board: boardName, ConsoleBaud: consoleBaud, KernelParams: cfg.KernelParamString()})
	if err != nil {
		return nil, fmt.Errorf("rendering cmdline.txt: %w", err)
	}
	files["cmdline.txt"] = strings.NewReader(cmdlineTxt)

	if art.Initramfs == nil {
		return nil, fmt.Errorf("pi-cm4 BootFiles: no initramfs archive was provided by the build pipeline")
	}
	files[initramfsName] = art.Initramfs

	return files, nil
}

// RawWrites implements boards.Board: the Pi boots via the GPU ROM and FAT
// partition alone, with no bootloader in the unpartitioned gap.
func (board) RawWrites(boards.Artifacts) []image.RawWrite { return nil }

// FirmwareFiles implements boards.Board: none. This module has no WiFi/BT
// hardware, so there's no firmware to ship into the initramfs.
func (board) FirmwareFiles(boards.Artifacts) map[string]io.Reader { return nil }

// UsbGadgetSupport implements boards.Board: unsupported, but explicitly
// uncharacterized rather than a proven hardware limitation (contrast
// pi-3b's UsbGadgetSupport, which names a specific hardware fact). The
// CM4's dwc2 controller is a real, kernel-compiled-in dual-role
// controller (see build/boards/pi-cm4/kernel.fragment) - unlike pi-3b's
// hub-wired port, there is no known reason gadget mode can't work here.
// What's missing is verification that Turing Pi 2's node carrier actually
// routes the SoC's USB2 signals to an accessible port (epic gosd-7676's
// deliberate "?" decision - see bean gosd-5trv for when/if this gets
// characterized). Refusing by default is the safe choice: claiming support
// that doesn't exist fails at runtime on-device, which is strictly worse
// than a --usb-gadget build-time refusal.
func (board) UsbGadgetSupport() boards.GadgetSupport {
	return boards.GadgetSupport{
		Supported: false,
		Reason:    "USB gadget mode hasn't been characterized on this carrier yet (Turing Pi 2's node wiring is unverified, not a known hardware limitation) - see bean gosd-5trv",
	}
}

// ConsoleBaudSupport implements boards.Board: supported. cmdline.txt's
// console= argument carries the baud rate BootFiles renders it with.
func (board) ConsoleBaudSupport() boards.ConsoleBaudSupport {
	return boards.ConsoleBaudSupport{Supported: true}
}

// EXT4Support implements boards.Board: supported. This board's kernel
// fragment asserts CONFIG_EXT4_FS=y from day one (see kernel.fragment).
func (board) EXT4Support() boards.EXT4Support {
	return boards.EXT4Support{Supported: true}
}
