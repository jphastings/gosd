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

	// kernelArtifactName is the artifact the pipeline must resolve for
	// the kernel image config.txt names ("kernel=kernel8.img"). It has no
	// per-file pinned URL (ArtifactRef.URL is empty): it's one of the
	// files GoSD compiles itself, resolved either from --artifacts-dir or,
	// falling back, from the CI-built artifact release (see bean
	// gosd-wtpa and internal/artifacts).
	kernelArtifactName = "kernel8.img"

	// initramfsName is the file name the initramfs is written under in
	// the FAT boot partition; config.txt's "initramfs" directive and
	// this name must match.
	initramfsName = "initramfs.cpio.zst"
)

type board struct{}

// New returns the pi-zero-2w Board implementation.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// Artifacts implements boards.Board: the kernel (not yet automatically
// fetchable), the GPU boot firmware, and the WiFi firmware blobs pinned in
// manifest.json.
func (board) Artifacts() []boards.ArtifactRef {
	m := manifest.Load()

	refs := make([]boards.ArtifactRef, 0, 1+len(m.BootFiles.Files)+len(m.WifiFirmware.Files))
	refs = append(refs, boards.ArtifactRef{Name: kernelArtifactName})
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

// BootFiles implements boards.Board: the kernel, GPU boot firmware,
// rendered config.txt/cmdline.txt, and the initramfs the build pipeline has
// already built into art.Initramfs.
func (board) BootFiles(_ boards.BuildConfig, art boards.Artifacts) (map[string]io.Reader, error) {
	m := manifest.Load()

	files := make(map[string]io.Reader, len(m.BootFiles.Files)+3)

	kernel, err := art.Open(kernelArtifactName)
	if err != nil {
		return nil, err
	}
	files[kernelArtifactName] = kernel

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

	cmdlineTxt, err := templates.RenderCmdlineTxt(templates.CmdlineTxtData{Board: boardName})
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
