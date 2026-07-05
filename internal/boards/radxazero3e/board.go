// Package radxazero3e implements internal/boards.Board for the Radxa Zero
// 3E: raw bootloader writes (idbloader.img, u-boot.itb) into the
// unpartitioned gap ahead of the FAT boot partition, and the kernel, DTB,
// initramfs, and extlinux.conf U-Boot reads from that partition. The
// bootloader and kernel artifacts are built by
// build/boards/radxa-zero-3e/{uboot,kernel}/build.sh; they have no per-file
// pinned URL, so they're resolved from --artifacts-dir or, falling back,
// from the CI-built artifact release (see bean gosd-wtpa and
// internal/artifacts). Locked byte offsets and extlinux.conf content: see
// bean gosd-gbsz.
package radxazero3e

import (
	"bytes"
	"fmt"
	"io"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/radxazero3e/templates"
	"github.com/jphastings/gosd/internal/image"
)

const (
	// boardName is the --board flag value.
	boardName = "radxa-zero-3e"

	// Artifact names: the file names expected inside --artifacts-dir, and
	// inside the radxa-zero-3e CI-built artifact release tarball. None of
	// these have a per-file pinned URL, so ArtifactRef leaves URL/SHA256
	// empty for all of them, same as pi-zero-2w's kernel.
	idbloaderArtifactName = "idbloader.img"
	ubootArtifactName     = "u-boot.itb"
	kernelArtifactName    = "Image"
	dtbArtifactName       = "rk3566-radxa-zero-3e.dtb"

	// initramfsName is the file name the initramfs is written under in
	// the FAT boot partition; extlinux.conf's initrd directive and this
	// name must match.
	initramfsName = "initramfs.cpio.zst"

	// extlinuxConfPath is where extlinux.conf lives inside the FAT boot
	// partition; U-Boot's distro boot scripts look for it here.
	extlinuxConfPath = "extlinux/extlinux.conf"

	// idbloaderOffsetBytes and ubootOffsetBytes are the locked raw-write
	// offsets into the unpartitioned gap ahead of the boot partition:
	// LBA 64 and LBA 16384 at 512-byte sectors, respectively.
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

// New returns the radxa-zero-3e Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// Artifacts implements boards.Board: the bootloader and kernel files built
// by build/boards/radxa-zero-3e/{uboot,kernel}/build.sh. None has a
// per-file pinned URL; ResolveArtifacts resolves them from --artifacts-dir
// or, falling back, from the radxa-zero-3e CI-built artifact release.
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
// rendered from the locked template.
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
		return nil, fmt.Errorf("radxa-zero-3e BootFiles: no initramfs archive was provided by the build pipeline")
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
			"boards: radxa-zero-3e u-boot.itb is %d bytes, which would end at byte %d when written at "+
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
		panic(fmt.Sprintf("boards: reading radxa-zero-3e artifact %q: %v", name, err))
	}
	if c, ok := r.(io.Closer); ok {
		_ = c.Close()
	}
	return data
}

// FirmwareFiles implements boards.Board: empty map, per gosd-gbsz's locked
// decision that this board has no runtime-loaded firmware in v0.1.
func (board) FirmwareFiles(boards.Artifacts) map[string]io.Reader {
	return map[string]io.Reader{}
}
