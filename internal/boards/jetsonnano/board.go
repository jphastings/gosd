// Package jetsonnano implements internal/boards.Board for the original NVIDIA
// Jetson Nano developer kit (Tegra210; P3450 carrier, P3448-0000 module): the
// kernel, mainline DTB, initramfs, and extlinux.conf in the FAT boot
// partition, and nothing in the gap ahead of it. The bootloader is NVIDIA's
// own, in the module's QSPI-NOR (CBoot, then L4T U-Boot running ordinary
// distro boot), so RawWrites is empty and GoSD never fetches or ships
// anything NVIDIA built for it. The kernel and DTB are built by
// `gosd build-kernel --board jetson-nano` and resolved from --artifacts-dir
// or the CI-built artifact release; the XUSB firmware is pinned in
// build/boards/jetson-nano/manifest.json. Locked decisions: epic gosd-72je;
// bench facts: docs/development/jetson-nano.md.
package jetsonnano

import (
	"bytes"
	"fmt"
	"io"
	"path"

	manifest "github.com/jphastings/gosd/build/boards/jetson-nano"
	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/jetsonnano/templates"
	"github.com/jphastings/gosd/internal/image"
)

const (
	// boardName is the --board flag value.
	boardName = "jetson-nano"

	// Artifact names: the file names expected inside --artifacts-dir, and
	// inside the jetson-nano CI-built artifact release tarball. Neither has
	// a per-file pinned URL.
	kernelArtifactName = "Image"
	dtbArtifactName    = "tegra210-p3450-0000.dtb"

	// initramfsName is the file name the initramfs is written under in
	// the FAT boot partition; extlinux.conf's initrd directive and this
	// name must match.
	initramfsName = "initramfs.cpio.zst"

	// extlinuxConfPath is where extlinux.conf lives inside the FAT boot
	// partition; U-Boot's distro boot scripts look for it here.
	extlinuxConfPath = "extlinux/extlinux.conf"

	// defaultConsoleBaud is UART-A's rate, per tegra210-p3450-0000.dts's
	// stdout-path ("serial0:115200n8"). --console-baud overrides it.
	defaultConsoleBaud = 115200

	// displayName is this board's human-readable name (see
	// boards.Board.DisplayName).
	displayName = "NVIDIA Jetson Nano"
)

type board struct{}

// New returns the jetson-nano Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// DisplayName implements boards.Board.
func (board) DisplayName() string { return displayName }

// Arch implements boards.Board: the Tegra210's Cortex-A57s are arm64.
func (board) Arch() boards.Arch { return boards.Arch{GOARCH: "arm64"} }

// Artifacts implements boards.Board: the kernel and DTB (artifact-release
// resolved) plus the XUSB firmware pinned in manifest.json.
func (board) Artifacts() []boards.ArtifactRef {
	refs := []boards.ArtifactRef{
		{Name: kernelArtifactName},
		{Name: dtbArtifactName},
	}
	for _, f := range manifest.Load().Firmware.Files {
		refs = append(refs, boards.ArtifactRef{Name: f.Name, URL: f.URL, SHA256: f.SHA256})
	}
	return refs
}

// BootFiles implements boards.Board: the kernel, DTB, the initramfs the
// build pipeline has already built into art.Initramfs, and extlinux.conf
// rendered from the locked template. BuildConfig.UsbGadget is deliberately
// ignored: the micro-USB port is peripheral-only in the DT, so gadget mode
// needs no boot-time change.
func (board) BootFiles(cfg boards.BuildConfig, art boards.Artifacts) (map[string]io.Reader, error) {
	files := make(map[string]io.Reader, 4)

	for _, name := range []string{kernelArtifactName, dtbArtifactName} {
		r, err := art.Open(name)
		if err != nil {
			return nil, err
		}
		files[name] = r
	}

	if art.Initramfs == nil {
		return nil, fmt.Errorf("jetson-nano BootFiles: no initramfs archive was provided by the build pipeline")
	}
	files[initramfsName] = art.Initramfs

	consoleBaud := cfg.ConsoleBaud
	if consoleBaud == 0 {
		consoleBaud = defaultConsoleBaud
	}
	extlinuxConf, err := templates.RenderExtlinuxConf(templates.ExtlinuxConfData{ConsoleBaud: consoleBaud, KernelParams: cfg.KernelParamString()})
	if err != nil {
		return nil, fmt.Errorf("rendering extlinux.conf: %w", err)
	}
	files[extlinuxConfPath] = bytes.NewReader([]byte(extlinuxConf))

	return files, nil
}

// RawWrites implements boards.Board: none. NVIDIA's boot chain lives in the
// module's own QSPI-NOR, so nothing goes in the gap ahead of the boot
// partition.
func (board) RawWrites(boards.Artifacts) []image.RawWrite { return nil }

// FirmwareFiles implements boards.Board: the XUSB firmware xhci-tegra loads
// at probe, at the path it requests (nvidia/tegra210/xusb.bin). Every USB
// port on the Nano is behind XUSB, so without it there is no USB host.
func (board) FirmwareFiles(art boards.Artifacts) map[string]io.Reader {
	fw := manifest.Load().Firmware

	files := make(map[string]io.Reader, len(fw.Files))
	for _, f := range fw.Files {
		files[path.Join(fw.DestDir, f.Name)] = art.MustOpen(f.Name)
	}
	return files
}

// UsbGadgetSupport implements boards.Board: supported, not yet
// hardware-verified. tegra210-p3450-0000.dts enables the XUDC
// (usb@700d0000) on the micro-USB port, whose padctl port usb2-0 is
// mode "peripheral" with usb-role-switch and a VBUS-sensing connector; the
// arm64 defconfig's tegra-xudc, usb-conn-gpio and configfs gadget stack all
// land =y in the fleet's no-modules build (see bean gosd-cxio's findings).
func (board) UsbGadgetSupport() boards.GadgetSupport {
	return boards.GadgetSupport{Supported: true}
}

// ConsoleBaudSupport implements boards.Board: supported. extlinux.conf's
// console= argument carries the baud rate BootFiles renders it with.
func (board) ConsoleBaudSupport() boards.ConsoleBaudSupport {
	return boards.ConsoleBaudSupport{Supported: true}
}

// EXT4Support implements boards.Board: supported. This board's stock kernel
// builds CONFIG_EXT4_FS=y (part of the arm64 defconfig baseline, same as
// the rest of the mainline fleet), so the data partition can mount ext4
// when --data-filesystem=ext4 is passed.
func (board) EXT4Support() boards.EXT4Support {
	return boards.EXT4Support{Supported: true}
}
