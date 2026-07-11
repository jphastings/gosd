// Package qemuvirt implements internal/boards.Board for qemu-virt: an
// internal-only profile (registered via boards.RegisterInternal, never
// offered as a default build target or in end-user docs - see the
// "qemu-virt board" locked decision in CLAUDE.md) targeting
// `qemu-system-aarch64 -M virt` with virtio devices, used for CI and local
// testing (see bean gosd-c54j and its children).
//
// Unlike the Pi and Radxa boards, qemu's `-M virt` machine has no GPU ROM or
// on-disk bootloader to hand off to: the runner invokes qemu with
// -kernel/-initrd directly, so this board needs no config.txt or
// extlinux.conf. The FAT boot partition still carries the kernel image,
// initramfs, and gosd.toml (added by the build pipeline for every board), so
// the same provisioning story (gosd.toml, cloud-init files) stays testable
// under qemu exactly as it works on real hardware.
//
// SD-card access on this machine is virtio-blk, which the kernel exposes as
// /dev/vda rather than /dev/mmcblkN; see gosd-init's boot-partition and
// data-partition device-candidate lists (cmd/gosd-init/main.go) for the
// corresponding /dev/vda1 / /dev/vda2 entries.
package qemuvirt

import (
	"fmt"
	"io"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/image"
)

const (
	// boardName is the --board flag value.
	boardName = "qemu-virt"

	// kernelArtifactName is the artifact the pipeline must resolve for the
	// kernel image this board's BootFiles writes to the boot partition. It
	// has no per-file pinned URL (ArtifactRef.URL is empty): it's compiled
	// by `gosd build-kernel --board qemu-virt` (bean gosd-07fl) and
	// resolved from --artifacts-dir or, falling back, the CI-built
	// artifact release - same pattern as pi-zero-2w's kernel8.img and
	// radxa-zero-3e's Image.
	kernelArtifactName = "Image"

	// initramfsName is the file name the initramfs is written under in
	// the FAT boot partition; the qemu runner's -initrd argument must
	// name this same file.
	initramfsName = "initramfs.cpio.zst"
)

type board struct{}

// New returns the qemu-virt Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// Arch implements boards.Board: qemu-virt only ever runs under
// qemu-system-aarch64, so it's always arm64.
func (board) Arch() boards.Arch { return boards.Arch{GOARCH: "arm64"} }

// Artifacts implements boards.Board: just the kernel image. qemu-virt has no
// GPU firmware or bootloader blobs to fetch - qemu boots the kernel
// directly.
func (board) Artifacts() []boards.ArtifactRef {
	return []boards.ArtifactRef{{Name: kernelArtifactName}}
}

// BootFiles implements boards.Board: the kernel image and the initramfs the
// build pipeline has already built into art.Initramfs. No config.txt or
// extlinux.conf - qemu is invoked with -kernel/-initrd directly, so there's
// no on-device bootloader to configure (see the package doc comment).
// gosd.toml is added by the build pipeline for every board, so it still
// lands at the FAT root here too.
func (board) BootFiles(_ boards.BuildConfig, art boards.Artifacts) (map[string]io.Reader, error) {
	files := make(map[string]io.Reader, 2)

	kernel, err := art.Open(kernelArtifactName)
	if err != nil {
		return nil, err
	}
	files[kernelArtifactName] = kernel

	if art.Initramfs == nil {
		return nil, fmt.Errorf("qemu-virt BootFiles: no initramfs archive was provided by the build pipeline")
	}
	files[initramfsName] = art.Initramfs

	return files, nil
}

// RawWrites implements boards.Board: qemu-virt has no bootloader in the
// unpartitioned gap ahead of the boot partition.
func (board) RawWrites(boards.Artifacts) []image.RawWrite { return nil }

// FirmwareFiles implements boards.Board: virtio devices are handled by the
// mainline kernel's built-in drivers, so there's no runtime-loaded firmware
// to place under /lib/firmware.
func (board) FirmwareFiles(boards.Artifacts) map[string]io.Reader {
	return map[string]io.Reader{}
}
