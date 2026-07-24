// Package nanopizero2 implements internal/boards.Board for the FriendlyElec
// NanoPi Zero2 (Rockchip RK3528A): raw bootloader writes (idbloader.img,
// u-boot.itb) into the unpartitioned gap ahead of the FAT boot partition, and
// the kernel, DTB, initramfs, and extlinux.conf U-Boot reads from that
// partition — the same boot chain as the Radxa Zero 3E (see
// internal/boards/radxazero3e), since both are Rockchip SoCs booted via
// idbloader + U-Boot proper + extlinux.
//
// INTERNAL-ONLY FOR NOW: this board is registered via boards.RegisterInternal
// (see cmd/gosd/build.go), not boards.Register, per bean gosd-wskc's scope
// amendment (2026-07-06). Mainline U-Boot support for this board
// (configs/nanopi-zero2-rk3528_defconfig) lands in v2026.07, which hadn't
// shipped a final release yet when this profile was written (bean gosd-f39b
// pins a v2026.07-rc tag to unblock hardware bring-up); there is no
// artifacts-pipeline job producing idbloader.img/u-boot.itb, so a real (non
// --artifacts-dir) build would 404 fetching them. Flip this to a public
// boards.Register call — and add the corresponding U-Boot job to
// .github/workflows/build-artifacts.yml + the manifest — once gosd-f39b
// completes and an artifact release contains this board's U-Boot files. See
// the checklist item on bean gosd-wskc.
//
// The bootloader and kernel artifacts are built by
// build/boards/nanopi-zero2/{uboot,kernel}/build.sh; they have no per-file
// pinned URL, so they're resolved from --artifacts-dir or, falling back,
// from the CI-built artifact release (see internal/artifacts), same as the
// Radxa Zero 3E. Locked byte offsets, extlinux.conf content, and the
// console=ttyS0,1500000n8 debug UART (verified against the mainline
// rk3528-nanopi-zero2.dts aliases node and rk3528.dtsi's uart0 node — see
// internal/boards/nanopizero2/templates): see bean gosd-wskc.
package nanopizero2

import (
	"bytes"
	"fmt"
	"io"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/nanopizero2/templates"
	"github.com/jphastings/gosd/internal/image"
)

const (
	// boardName is the --board flag value.
	boardName = "nanopi-zero2"

	// Artifact names: the file names expected inside --artifacts-dir, and
	// inside the nanopi-zero2 CI-built artifact release tarball. None of
	// these have a per-file pinned URL, so ArtifactRef leaves URL/SHA256
	// empty for all of them, same as radxa-zero-3e's.
	idbloaderArtifactName = "idbloader.img"
	ubootArtifactName     = "u-boot.itb"
	kernelArtifactName    = "Image"
	dtbArtifactName       = "rk3528-nanopi-zero2.dtb"

	// initramfsName is the file name the initramfs is written under in
	// the FAT boot partition; extlinux.conf's initrd directive and this
	// name must match.
	initramfsName = "initramfs.cpio.zst"

	// extlinuxConfPath is where extlinux.conf lives inside the FAT boot
	// partition; U-Boot's distro boot scripts look for it here.
	extlinuxConfPath = "extlinux/extlinux.conf"

	// idbloaderOffsetBytes and ubootOffsetBytes are the locked raw-write
	// offsets into the unpartitioned gap ahead of the boot partition:
	// LBA 64 and LBA 16384 at 512-byte sectors, respectively - the same
	// Rockchip layout as the Radxa Zero 3E.
	idbloaderOffsetBytes = 32768
	ubootOffsetBytes     = 8388608

	// maxUbootEndBytes is the byte the boot partition starts at (16MiB);
	// u-boot.itb must end at or before it. internal/image.Write enforces
	// this too (a raw write can't overlap partition 1), but that guard
	// fires late and reports the collision in image-layout terms; this
	// check fires first with a message about the artifact itself.
	maxUbootEndBytes = 16 * 1024 * 1024
)

type board struct{}

// New returns the nanopi-zero2 Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// Arch implements boards.Board: the RK3528A is arm64-only.
func (board) Arch() boards.Arch { return boards.Arch{GOARCH: "arm64"} }

// Artifacts implements boards.Board: the bootloader and kernel files built
// by build/boards/nanopi-zero2/{uboot,kernel}/build.sh. None has a per-file
// pinned URL; ResolveArtifacts resolves them from --artifacts-dir or,
// falling back, from the nanopi-zero2 CI-built artifact release.
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
// ignored: this board has no USB controller in any numbered mainline
// kernel release as of the pinned kernel tag (see bean gosd-cwjf's "USB gate"
// finding), so there is no boot-time gadget change to make here yet - see
// UsbGadgetSupport, which is what actually stops `gosd build --usb-gadget`
// from selecting this board before BootFiles is ever called.
func (board) BootFiles(_ boards.BuildConfig, art boards.Artifacts) (map[string]io.Reader, error) {
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
		return nil, fmt.Errorf("nanopi-zero2 BootFiles: no initramfs archive was provided by the build pipeline")
	}
	files[initramfsName] = art.Initramfs

	extlinuxConf, err := templates.RenderExtlinuxConf()
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
// with an actionable message, same as Artifacts.MustOpen's convention for
// board-package invariant violations.
func (board) RawWrites(art boards.Artifacts) []image.RawWrite {
	idbloader := mustReadArtifact(art, idbloaderArtifactName)
	uboot := mustReadArtifact(art, ubootArtifactName)

	if end := int64(ubootOffsetBytes) + int64(len(uboot)); end > maxUbootEndBytes {
		panic(fmt.Sprintf(
			"boards: nanopi-zero2 u-boot.itb is %d bytes, which would end at byte %d when written at "+
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
		panic(fmt.Sprintf("boards: reading nanopi-zero2 artifact %q: %v", name, err))
	}
	if c, ok := r.(io.Closer); ok {
		_ = c.Close()
	}
	return data
}

// FirmwareFiles implements boards.Board: empty map. This board's GbE
// (stmmac/dwmac-rk + Realtek RTL8211F) and storage (dw_mmc/dwcmshc) drivers
// need no runtime-loaded firmware, per bean gosd-vcae's findings.
func (board) FirmwareFiles(boards.Artifacts) map[string]io.Reader {
	return map[string]io.Reader{}
}

// UsbGadgetSupport implements boards.Board: unsupported. The RK3528 has no
// USB controller device-tree node in any numbered mainline kernel release as
// of the pinned kernel tag (see COMPATIBILITY.md's [^nanopi-usb] footnote and
// bean gosd-vcae's findings) - there's no UDC for the gadget package to bind
// to at all, host or peripheral. This lands with a future fleet-wide kernel
// bump (bean gosd-vcae), never a single-board one (see CLAUDE.md's "same
// kernel tag" locked decision).
func (board) UsbGadgetSupport() boards.GadgetSupport {
	return boards.GadgetSupport{
		Supported: false,
		Reason:    "the RK3528 has no USB controller device-tree node at the pinned kernel tag (host or peripheral); tracked by bean gosd-vcae, arrives with a future fleet-wide kernel bump",
	}
}
