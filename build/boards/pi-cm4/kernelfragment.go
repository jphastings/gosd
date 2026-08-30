package manifest

import _ "embed"

// KernelFragment is the GoSD Kconfig fragment merged onto bcm2711_defconfig
// (via scripts/kconfig/merge_config.sh) to build this board's trimmed
// kernel — see internal/kernelspec, the Go-native source of truth for
// kernel build inputs. It's embedded here, alongside manifest.json, rather
// than under internal/kernelspec itself, because go:embed can only reach
// files inside its own package directory.
//
//go:embed kernel.fragment
var KernelFragment []byte
