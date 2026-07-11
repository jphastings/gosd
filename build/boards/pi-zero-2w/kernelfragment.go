package manifest

import _ "embed"

// KernelFragment is the GoSD Kconfig fragment merged onto bcm2711_defconfig
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
