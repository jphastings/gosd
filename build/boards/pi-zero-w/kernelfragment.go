package manifest

import "embed"

// KernelFragment is the GoSD Kconfig fragment merged onto bcmrpi_defconfig
// (via scripts/kconfig/merge_config.sh) to build this board's trimmed
// kernel — see internal/kernelspec, the Go-native source of truth for
// kernel build inputs (bean gosd-di6v), which embeds this via this package.
// It's embedded here, alongside manifest.json, rather than under
// internal/kernelspec itself, because go:embed can only reach files inside
// its own package directory. The board's build.sh used to read
// kernel.fragment directly from disk too, until bean gosd-07fl retired it
// in favor of gosd build-kernel reading internal/kernelspec directly.
//
//go:embed kernel.fragment
var KernelFragment []byte

// PatchesFS embeds this board's device-tree patches, applied in filename
// order with `patch -p1` before the config step (see internal/kernelbuild).
// The mainline-style bcm2835 DTs in the raspberrypi/linux tree lack the
// peripheral dma-ranges window the tree's downstream slave-DMA convention
// requires, which broke all SD-card I/O on real hardware — bean gosd-1ey5
// has the full analysis.
//
//go:embed kernel/patches
var PatchesFS embed.FS
