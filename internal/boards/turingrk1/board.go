// Package turingrk1 implements internal/boards.Board for the Turing RK1
// (Rockchip RK3588): raw bootloader writes (idbloader.img, u-boot.itb) into
// the unpartitioned gap ahead of the FAT boot partition, and the kernel,
// DTB, initramfs, and extlinux.conf U-Boot reads from that partition. This
// board has no SD/microSD slot at all — it boots from onboard eMMC only —
// but the RK3588 BootROM/U-Boot chain doesn't care what block device it's
// written to, so the image this profile produces is the same raw-disk
// shape as every other Rockchip board's; only the end-user delivery
// mechanism differs (see docs/turing-rk1-flashing.md). The bootloader and
// kernel artifacts are built by build/boards/turing-rk1/{uboot,kernel}; they
// have no per-file pinned URL, so they're resolved from --artifacts-dir or,
// falling back, from the CI-built artifact release (see
// internal/artifacts). Locked byte offsets and research findings: see bean
// gosd-k4w2 and gosd-jvtg.
package turingrk1

import (
	"bytes"
	"fmt"
	"io"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/turingrk1/templates"
	"github.com/jphastings/gosd/internal/image"
)

const (
	// boardName is the --board flag value.
	boardName = "turing-rk1"

	// Artifact names: the file names expected inside --artifacts-dir, and
	// inside the turing-rk1 CI-built artifact release tarball. None of
	// these have a per-file pinned URL, so ArtifactRef leaves URL/SHA256
	// empty for all of them, same as the rest of the Rockchip fleet.
	idbloaderArtifactName = "idbloader.img"
	ubootArtifactName     = "u-boot.itb"
	kernelArtifactName    = "Image"
	dtbArtifactName       = "rk3588-turing-rk1.dtb"

	// initramfsName is the file name the initramfs is written under in
	// the FAT boot partition; extlinux.conf's initrd directive and this
	// name must match.
	initramfsName = "initramfs.cpio.zst"

	// extlinuxConfPath is where extlinux.conf lives inside the FAT boot
	// partition; U-Boot's distro boot scripts look for it here.
	extlinuxConfPath = "extlinux/extlinux.conf"

	// idbloaderOffsetBytes and ubootOffsetBytes are the locked raw-write
	// offsets into the unpartitioned gap ahead of the boot partition:
	// LBA 64 and LBA 16384 at 512-byte sectors, respectively — the same
	// offsets the rest of the Rockchip fleet uses (U-Boot's own
	// doc/board/rockchip/rockchip.rst gives seek=64 for the first-stage
	// loader across the whole family, RK3588 included; see bean
	// gosd-k4w2's research).
	idbloaderOffsetBytes = 32768
	ubootOffsetBytes     = 8388608

	// maxUbootEndBytes is the byte the boot partition starts at (16MiB);
	// u-boot.itb must end at or before it. internal/image.Write enforces
	// this too, but that guard fires late and reports the collision in
	// image-layout terms; this check fires first with a message about the
	// artifact itself.
	maxUbootEndBytes = 16 * 1024 * 1024

	// defaultConsoleBaud is this board's own console rate: 115200, per
	// rk3588-turing-rk1.dts's stdout-path (bean gosd-k4w2's research) —
	// NOT the 1.5M baud the rest of the Rockchip fleet defaults to.
	// --console-baud overrides it.
	defaultConsoleBaud = 115200

	// displayName is this board's human-readable name (see
	// boards.Board.DisplayName), matching COMPATIBILITY.md's prose.
	displayName = "Turing RK1"
)

type board struct{}

// New returns the turing-rk1 Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// DisplayName implements boards.Board.
func (board) DisplayName() string { return displayName }

// Arch implements boards.Board: the RK3588 is arm64.
func (board) Arch() boards.Arch { return boards.Arch{GOARCH: "arm64"} }

// Artifacts implements boards.Board: the bootloader and kernel files built
// by build/boards/turing-rk1/{uboot,kernel}. None has a per-file pinned
// URL; ResolveArtifacts resolves them from --artifacts-dir or, falling
// back, from the turing-rk1 CI-built artifact release.
func (board) Artifacts() []boards.ArtifactRef {
	return []boards.ArtifactRef{
		{Name: idbloaderArtifactName},
		{Name: ubootArtifactName},
		{Name: kernelArtifactName},
		{Name: dtbArtifactName},
	}
}

// BootFiles implements boards.Board: the kernel, DTB, the initramfs the
// build pipeline has already built into art.Initramfs, and extlinux.conf
// rendered from the locked template. BuildConfig.UsbGadget is deliberately
// ignored: like the rest of the fleet's DWC3-based boards, this board's
// OTG-capable USB2 PHY (u2phy0_otg) doesn't need a boot-time change to make
// gadget mode available.
func (board) BootFiles(cfg boards.BuildConfig, art boards.Artifacts) (map[string]io.Reader, error) {
	files := make(map[string]io.Reader, 4)

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

	if art.Initramfs == nil {
		return nil, fmt.Errorf("turing-rk1 BootFiles: no initramfs archive was provided by the build pipeline")
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

// RawWrites implements boards.Board: idbloader.img and u-boot.itb, written
// into the unpartitioned gap at their locked offsets. Both artifacts are
// read in full up front (rather than streamed) so u-boot.itb's size can be
// checked against the 16MiB boot-partition start before the image writer
// ever sees it - RawWrites can't return an error, so a violation panics
// with an actionable message, same as the rest of the fleet's convention.
func (board) RawWrites(art boards.Artifacts) []image.RawWrite {
	idbloader := mustReadArtifact(art, idbloaderArtifactName)
	uboot := mustReadArtifact(art, ubootArtifactName)

	if end := int64(ubootOffsetBytes) + int64(len(uboot)); end > maxUbootEndBytes {
		panic(fmt.Sprintf(
			"boards: turing-rk1 u-boot.itb is %d bytes, which would end at byte %d when written at "+
				"offset %d; it must end at or before byte %d (16MiB, where the boot partition starts) - "+
				"rebuild u-boot.itb smaller (e.g. drop unused U-Boot drivers/configs) or the board's locked "+
				"raw-write layout needs revisiting",
			len(uboot), end, ubootOffsetBytes, maxUbootEndBytes))
	}

	return []image.RawWrite{
		{OffsetBytes: idbloaderOffsetBytes, Content: bytes.NewReader(idbloader)},
		{OffsetBytes: ubootOffsetBytes, Content: bytes.NewReader(uboot)},
	}
}

// mustReadArtifact opens and fully reads the artifact named name, closing it
// afterward. It panics on failure: name is always one this board declared
// in Artifacts(), so a failure here means the pipeline didn't resolve it
// before calling RawWrites, which is a programmer error, not a runtime one.
func mustReadArtifact(art boards.Artifacts, name string) []byte {
	r := art.MustOpen(name)
	data, err := io.ReadAll(r)
	if err != nil {
		panic(fmt.Sprintf("boards: reading turing-rk1 artifact %q: %v", name, err))
	}
	if c, ok := r.(io.Closer); ok {
		_ = c.Close()
	}
	return data
}

// FirmwareFiles implements boards.Board: empty map -- this board has no
// runtime-loaded firmware in scope for this bring-up.
func (board) FirmwareFiles(boards.Artifacts) map[string]io.Reader {
	return map[string]io.Reader{}
}

// UsbGadgetSupport implements boards.Board: supported. rk3588-turing-rk1's
// u2phy0_otg (USB2 PHY) is status "okay" in the mainline DT and the
// defconfig compiles in USB_GADGET/USB_DWC3_DUAL_ROLE (bean gosd-k4w2's
// research) -- not yet hardware-verified.
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
// the rest of the fleet), so the data partition can mount ext4 when
// --data-filesystem=ext4 is passed.
func (board) EXT4Support() boards.EXT4Support {
	return boards.EXT4Support{Supported: true}
}
