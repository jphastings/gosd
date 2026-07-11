package kernelbuild

import "github.com/jphastings/gosd/internal/kernelspec"

// Overlay is a developer's additional kernel customization, layered on top
// of a board's kernelspec.KernelSpec: an extra Kconfig fragment merged onto
// the .config after GoSD's own ConfigFragment, and extra device-tree patches
// applied after every one of KernelSpec.DTSPatches.
//
// Parsing gosd-kernel.toml into an Overlay is bean gosd-hkp7's job; this
// package only consumes already-parsed values. The zero value is a no-op
// overlay (a plain GoSD build with no developer customization).
type Overlay struct {
	// ConfigFragment is merged onto the kernel's .config via
	// scripts/kconfig/merge_config.sh -m, after KernelSpec.ConfigFragment
	// has already been merged. Empty means no overlay fragment.
	ConfigFragment []byte
	// Patches are applied with `patch -p1`, in slice order, after every
	// patch in KernelSpec.DTSPatches has already been applied.
	Patches []kernelspec.Patch
}
