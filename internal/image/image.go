// Package image defines the contract for assembling a bootable SD-card
// image from cross-compiled binaries. The real implementation (partitioning,
// FAT boot partition, kernel/U-Boot placement, initramfs embedding) belongs
// to later beans; this package only stubs the interface so the CLI's build
// pipeline has a well-defined final step to call.
package image

import (
	"context"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/initramfs"
)

// Spec describes everything needed to assemble one board's image.
type Spec struct {
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

// Assembler turns a Spec into a flashable .img file at Spec.OutputPath.
type Assembler interface {
	Assemble(ctx context.Context, spec Spec) error
}

// NotImplemented is the Assembler used until a real implementation lands.
// It always fails clearly rather than silently producing a broken image.
type NotImplemented struct{}

// Assemble implements Assembler.
func (NotImplemented) Assemble(ctx context.Context, spec Spec) error {
	return errNotImplemented
}
