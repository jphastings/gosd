// Package initramfs defines the contract for building the initramfs cpio
// archive that gosd-init and the boot process run out of. The real
// implementation (packing gosd-init, kernel modules, and firmware into a
// cpio archive) belongs to a later bean; this package only stubs the
// interface so internal/image has something to build against today.
package initramfs

import "context"

// Spec describes the inputs needed to build an initramfs for one board.
type Spec struct {
	// InitBinaryPath is the cross-compiled gosd-init binary to embed as
	// /init in the archive.
	InitBinaryPath string
	// OutputDir is where the resulting cpio archive should be written.
	OutputDir string
}

// Builder produces an initramfs archive from a Spec. Implementations
// return the path to the archive they wrote.
type Builder interface {
	Build(ctx context.Context, spec Spec) (path string, err error)
}

// NotImplemented is the Builder used until a real implementation lands.
// It always fails clearly rather than silently producing a broken image.
type NotImplemented struct{}

// Build implements Builder.
func (NotImplemented) Build(ctx context.Context, spec Spec) (string, error) {
	return "", errNotImplemented
}
