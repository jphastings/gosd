// Package radxazero3e is a registered skeleton implementation of
// internal/boards.Board for the Radxa Zero 3E: enough to appear on --board
// and in listings, but its boot files are not implemented yet. The
// bootloader raw-writes (idbloader.img, u-boot.itb) and the extlinux.conf
// template belong to bean gosd-gbsz, not this one.
package radxazero3e

import (
	"errors"
	"io"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/image"
)

// boardName is the --board flag value.
const boardName = "radxa-zero-3e"

// errNotImplemented is returned by BootFiles until gosd-gbsz lands.
var errNotImplemented = errors.New(
	"radxa-zero-3e boot files (extlinux.conf, kernel, dtb, initramfs) are not implemented yet; " +
		"they land with bean gosd-gbsz")

type board struct{}

// New returns the radxa-zero-3e Board skeleton.
func New() boards.Board { return board{} }

// Name implements boards.Board.
func (board) Name() string { return boardName }

// Artifacts implements boards.Board. Empty until gosd-gbsz (blocked on the
// U-Boot and kernel builds) wires up the real artifact list.
func (board) Artifacts() []boards.ArtifactRef { return nil }

// BootFiles implements boards.Board. Always fails clearly rather than
// silently producing an unbootable image.
func (board) BootFiles(boards.BuildConfig, boards.Artifacts) (map[string]io.Reader, error) {
	return nil, errNotImplemented
}

// RawWrites implements boards.Board. Empty until gosd-gbsz wires up
// idbloader.img/u-boot.itb.
func (board) RawWrites(boards.Artifacts) []image.RawWrite { return nil }

// FirmwareFiles implements boards.Board: empty map, per gosd-gbsz's locked
// decision that this board has no runtime-loaded firmware in v0.1.
func (board) FirmwareFiles(boards.Artifacts) map[string]io.Reader {
	return map[string]io.Reader{}
}
