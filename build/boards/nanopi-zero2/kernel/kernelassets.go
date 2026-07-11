// Package kernelassets embeds the NanoPi Zero2's GoSD Kconfig fragment and
// device-tree patches so internal/kernelspec's Go-native KernelSpec (bean
// gosd-di6v) can be the single source of truth for kernel build inputs. The
// embed directive can only reach files inside its own package directory,
// which is why this package lives alongside kernel-fragment.config and
// patches/ rather than under internal/kernelspec itself; docker-build.sh
// keeps reading those same files directly from disk, unchanged, until bean
// gosd-07fl retires it.
package kernelassets

import "embed"

//go:embed kernel-fragment.config
var ConfigFragment []byte

// PatchesFS embeds every device-tree patch applied (in filename order,
// `patch -p1`) before the config step. See docker-build.sh's "Applying
// GoSD device-tree patches" loop.
//
//go:embed patches
var PatchesFS embed.FS
