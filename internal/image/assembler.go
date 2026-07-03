package image

import (
	"context"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/initramfs"
)

// AssembleSpec describes everything needed to assemble one board's image
// from a full gosd build (cross-compiled binaries, initramfs, board
// metadata). It is the input to Assembler, the CLI's build-pipeline
// interface; contrast with Spec, the lower-level input to Write.
type AssembleSpec struct {
	Board Board

	// AppBinaryPath is the cross-compiled user application.
	AppBinaryPath string
	// InitBinaryPath is the cross-compiled gosd-init binary.
	InitBinaryPath string
	// Initramfs builds the initramfs archive that embeds InitBinaryPath;
	// the assembler is expected to call it as part of assembly.
	Initramfs initramfs.Builder

	// Hostname is the device hostname to bake into the image.
	Hostname string
	// WifiSSID and WifiPassword are baked-in WPA2-PSK/open credentials,
	// per the v0.1 "baked credentials" approach. Both empty means no
	// WiFi is configured.
	WifiSSID     string
	WifiPassword string

	// OutputPath is where the finished .img file should be written.
	OutputPath string
}

// Board is a local alias so callers of this package don't need a separate
// import for the handful of board fields an assembler needs.
type Board = boards.Board

// Assembler turns an AssembleSpec into a flashable .img file at
// AssembleSpec.OutputPath. The real implementation - feeding the initramfs
// and board-specific boot files into Write - belongs to a later bean
// (gosd-3zrc); NotImplemented stands in until then.
type Assembler interface {
	Assemble(ctx context.Context, spec AssembleSpec) error
}

// NotImplemented is the Assembler used until the full build pipeline is
// wired up. It always fails clearly rather than silently producing a
// broken image.
type NotImplemented struct{}

// Assemble implements Assembler.
func (NotImplemented) Assemble(ctx context.Context, spec AssembleSpec) error {
	return errNotImplemented
}
