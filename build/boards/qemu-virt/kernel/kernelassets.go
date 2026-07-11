// Package kernelassets embeds the qemu-virt board's GoSD Kconfig fragment so
// internal/kernelspec's Go-native KernelSpec (bean gosd-di6v) can be the
// single source of truth for kernel build inputs. go:embed can only reach
// files inside its own package directory, which is why this package lives
// alongside kernel-fragment.config rather than under internal/kernelspec
// itself. The board's docker-build.sh used to read the same file directly
// from disk too, until bean gosd-07fl retired it in favor of gosd
// build-kernel reading internal/kernelspec directly. Unlike the
// Rockchip-family boards, qemu-virt has no device-tree patches: qemu's -M
// virt machine supplies its own device tree, so there's no DTB to build or
// patch.
package kernelassets

import _ "embed"

//go:embed kernel-fragment.config
var ConfigFragment []byte
