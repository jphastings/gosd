// Package kernelassets embeds the Radxa ROCK 4SE's GoSD Kconfig fragment and
// device-tree patches so internal/kernelspec's Go-native KernelSpec (mirrors
// radxa-zero-3e, bean gosd-di6v) can be the single source of truth for kernel
// build inputs. The embed directive can only reach files inside its own
// package directory, which is why this package lives alongside
// kernel-fragment.config and patches/ rather than under internal/kernelspec
// itself. See build/boards/radxa-zero-3e/kernel/kernelassets.go for the
// template this mirrors.
package kernelassets

import "embed"

//go:embed kernel-fragment.config
var ConfigFragment []byte

// PatchesFS embeds every device-tree patch applied (in filename order,
// `patch -p1`) before the config step - see internal/kernelbuild's
// DTS-patch application step.
//
//go:embed patches
var PatchesFS embed.FS
