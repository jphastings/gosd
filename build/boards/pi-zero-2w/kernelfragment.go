package manifest

import _ "embed"

// KernelFragment is the GoSD Kconfig fragment merged onto bcm2711_defconfig
// (via scripts/kconfig/merge_config.sh) to build this board's trimmed
// kernel — see build.sh and internal/kernelspec, which is the Go-native
// source of truth for kernel build inputs (bean gosd-di6v). It's embedded
// here, alongside manifest.json, rather than under internal/kernelspec
// itself, because go:embed can only reach files inside its own package
// directory; build.sh keeps working unchanged and reads kernel.fragment
// directly from disk until bean gosd-07fl retires it.
//
//go:embed kernel.fragment
var KernelFragment []byte
