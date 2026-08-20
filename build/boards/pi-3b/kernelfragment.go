package manifest

import "embed"

// KernelFragment is the GoSD Kconfig fragment merged onto bcm2711_defconfig
// (via scripts/kconfig/merge_config.sh) to build this board's trimmed
// kernel — see internal/kernelspec, the Go-native source of truth for
// kernel build inputs (bean gosd-di6v), which embeds this via this package.
// It's embedded here, alongside manifest.json, rather than under
// internal/kernelspec itself, because go:embed can only reach files inside
// its own package directory.
//
//go:embed kernel.fragment
var KernelFragment []byte

// PatchesFS embeds this board's device-tree patches, applied in filename
// order with `patch -p1` before the config step (see internal/kernelbuild).
// Mirrors pi-zero-w's PatchesFS; see that package's doc comment for why the
// patches live under kernel/ rather than internal/kernelspec itself.
//
//go:embed kernel/patches
var PatchesFS embed.FS
