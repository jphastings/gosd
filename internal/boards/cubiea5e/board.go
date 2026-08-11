// Package cubiea5e implements internal/boards.Board for the Radxa Cubie A5E
// (Allwinner A527, sun55iw3 die): a single raw bootloader write
// (u-boot-sunxi-with-spl.bin) into the unpartitioned gap ahead of the FAT
// boot partition, and the kernel, DTB, initramfs, and extlinux.conf U-Boot
// reads from that partition.
//
// The sunxi boot chain is new to gosd - the Cubie A5E is its first Allwinner
// board - but reaches the same place as the Rockchip boards' two-stage
// chain by a shorter path. The Allwinner BootROM loads ONE file straight
// from a fixed SD-card byte offset (8KiB, sector 16): u-boot-sunxi-with-spl.bin,
// a FIT image U-Boot's own build packages from SPL, BL31 (compiled from a
// pinned TF-A fork - mainline Trusted Firmware-A has no sun55i_a523
// platform yet, see bean gosd-jpc8), U-Boot proper, and the board DTB - one
// RawWrite where the Rockchip boards need two (idbloader.img at LBA 64,
// u-boot.itb at LBA 16384). From there boot proceeds exactly like the
// Rockchip boards: U-Boot's distro boot scripts find extlinux/extlinux.conf
// on the FAT boot partition.
//
// The bootloader and kernel artifacts are built by
// build/boards/cubie-a5e/{uboot,kernel}; they have no per-file pinned URL,
// so they're resolved from --artifacts-dir or, falling back, from the
// CI-built artifact release (see internal/artifacts). Pinned values
// (offsets, console, artifact names) are bean gosd-jpc8's research findings.
// Public since the artifacts/v0.9.0 release (bean gosd-zh95's activation).
package cubiea5e

import (
	"bytes"
	"fmt"
	"io"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/cubiea5e/templates"
	"github.com/jphastings/gosd/internal/image"
)

const (
	// boardName is the --board flag value.
	boardName = "cubie-a5e"

	// Artifact names: the file names expected inside --artifacts-dir, and
	// inside the cubie-a5e CI-built artifact release tarball. None of
	// these has a per-file pinned URL, so ArtifactRef leaves URL/SHA256
	// empty for all of them.
	ubootArtifactName  = "u-boot-sunxi-with-spl.bin"
	kernelArtifactName = "Image"
	dtbArtifactName    = "sun55i-a527-cubie-a5e.dtb"

	// initramfsName is the file name the initramfs is written under in the
	// FAT boot partition; extlinux.conf's initrd directive and this name
	// must match.
	initramfsName = "initramfs.cpio.zst"

	// extlinuxConfPath is where extlinux.conf lives inside the FAT boot
	// partition; U-Boot's distro boot scripts look for it here.
	extlinuxConfPath = "extlinux/extlinux.conf"

	// ubootOffsetBytes is the locked raw-write offset into the
	// unpartitioned gap ahead of the boot partition: byte 8192 (8KiB,
	// sector 16), the sunxi BootROM's SD-card load address for the
	// A523/A527 family (bean gosd-jpc8's research, doc/board/allwinner/
	// sunxi.rst at U-Boot's pinned tag).
	ubootOffsetBytes = 8192

	// maxUbootEndBytes is the byte the boot partition starts at (16MiB);
	// u-boot-sunxi-with-spl.bin must end at or before it. internal/
	// image.Write enforces this too, but that guard fires late and
	// reports the collision in image-layout terms; this check fires
	// first with a message about the artifact itself.
	maxUbootEndBytes = 16 * 1024 * 1024

	// defaultConsoleBaud is this board's own console rate, used whenever
	// BuildConfig.ConsoleBaud is unset (0). The board DT's stdout-path is
	// serial0:115200n8 on uart0/ttyS0 (bean gosd-jpc8's research) - a
	// different UART and a much slower default than the Rockchip boards'
	// ttyS2 @ 1500000. --console-baud overrides it; see bean gosd-zp9s.
	defaultConsoleBaud = 115200

	// displayName is this board's human-readable name (see
	// boards.Board.DisplayName), matching COMPATIBILITY.md's prose.
	displayName = "Radxa Cubie A5E"
)

type board struct{}

// New returns the cubie-a5e Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// DisplayName implements boards.Board.
func (board) DisplayName() string { return displayName }

// Arch implements boards.Board: the A527 is arm64.
func (board) Arch() boards.Arch { return boards.Arch{GOARCH: "arm64"} }

// Artifacts implements boards.Board: the bootloader and kernel files built
// by build/boards/cubie-a5e/{uboot,kernel}. None has a per-file pinned URL;
// ResolveArtifacts resolves them from --artifacts-dir or, falling back, from
// the cubie-a5e CI-built artifact release.
func (board) Artifacts() []boards.ArtifactRef {
	return []boards.ArtifactRef{
		{Name: ubootArtifactName},
		{Name: kernelArtifactName},
		{Name: dtbArtifactName},
	}
}

// BootFiles implements boards.Board: the kernel, DTB, the initramfs the
// build pipeline has already built into art.Initramfs, and extlinux.conf
// rendered from the locked template. BuildConfig.UsbGadget is deliberately
// ignored: the board DT already pins usb_otg's dr_mode to "peripheral" at
// the pinned kernel (mainline MUSB, allwinner,sun8i-a33-musb - see bean
// gosd-jpc8), so the controller boots in peripheral mode regardless of
// --usb-gadget, and no boot-file change is needed.
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
		return nil, fmt.Errorf("cubie-a5e BootFiles: no initramfs archive was provided by the build pipeline")
	}
	files[initramfsName] = art.Initramfs

	consoleBaud := cfg.ConsoleBaud
	if consoleBaud == 0 {
		consoleBaud = defaultConsoleBaud
	}
	extlinuxConf, err := templates.RenderExtlinuxConf(templates.ExtlinuxConfData{ConsoleBaud: consoleBaud})
	if err != nil {
		return nil, fmt.Errorf("rendering extlinux.conf: %w", err)
	}
	files[extlinuxConfPath] = bytes.NewReader([]byte(extlinuxConf))

	return files, nil
}

// RawWrites implements boards.Board: u-boot-sunxi-with-spl.bin, written into
// the unpartitioned gap at its locked offset - the sunxi BootROM's single
// SPL+FIT load, unlike the Rockchip boards' idbloader+itb pair. The artifact
// is read in full up front (rather than streamed) so its size can be checked
// against the 16MiB boot-partition start before the image writer ever sees
// it - RawWrites can't return an error, so a violation panics with an
// actionable message, mirroring rock4se's convention.
func (board) RawWrites(art boards.Artifacts) []image.RawWrite {
	uboot := mustReadArtifact(art, ubootArtifactName)

	if end := int64(ubootOffsetBytes) + int64(len(uboot)); end > maxUbootEndBytes {
		panic(fmt.Sprintf(
			"boards: cubie-a5e u-boot-sunxi-with-spl.bin is %d bytes, which would end at byte %d when written "+
				"at offset %d; it must end at or before byte %d (16MiB, where the boot partition starts) - "+
				"rebuild u-boot-sunxi-with-spl.bin smaller (e.g. drop unused U-Boot drivers/configs) or the "+
				"board's locked raw-write layout needs revisiting",
			len(uboot), end, ubootOffsetBytes, maxUbootEndBytes))
	}

	return []image.RawWrite{
		{OffsetBytes: ubootOffsetBytes, Content: bytes.NewReader(uboot)},
	}
}

// mustReadArtifact opens and fully reads the artifact named name, closing it
// afterward. It panics on failure: name is always one this board declared in
// Artifacts(), so a failure here means the pipeline didn't resolve it before
// calling RawWrites, which is a programmer error, not a runtime one.
func mustReadArtifact(art boards.Artifacts, name string) []byte {
	r := art.MustOpen(name)
	data, err := io.ReadAll(r)
	if err != nil {
		panic(fmt.Sprintf("boards: reading cubie-a5e artifact %q: %v", name, err))
	}
	if c, ok := r.(io.Closer); ok {
		_ = c.Close()
	}
	return data
}

// FirmwareFiles implements boards.Board: empty map — this board has no
// runtime-loaded firmware in scope (onboard WiFi/BT is out of scope for the
// whole epic gosd-h1wv; see COMPATIBILITY.md).
func (board) FirmwareFiles(boards.Artifacts) map[string]io.Reader {
	return map[string]io.Reader{}
}

// UsbGadgetSupport implements boards.Board: supported. The board DT pins
// usb_otg's dr_mode to "peripheral" at the pinned kernel (mainline MUSB,
// bean gosd-jpc8), so the controller is already gadget-capable with no DTS
// patch needed - unlike most boards, this needed no research surprise to
// enable. Not yet hardware-verified (bench bean gosd-6pfn).
func (board) UsbGadgetSupport() boards.GadgetSupport {
	return boards.GadgetSupport{Supported: true}
}

// ConsoleBaudSupport implements boards.Board: supported. extlinux.conf's
// console= argument carries the baud rate BootFiles renders it with.
func (board) ConsoleBaudSupport() boards.ConsoleBaudSupport {
	return boards.ConsoleBaudSupport{Supported: true}
}

// EXT4Support implements boards.Board: supported. This board's stock kernel
// builds CONFIG_EXT4_FS=y (see COMPATIBILITY.md), so the data partition can mount
// ext4 when --data-filesystem=ext4 is passed.
func (board) EXT4Support() boards.EXT4Support {
	return boards.EXT4Support{Supported: true}
}
