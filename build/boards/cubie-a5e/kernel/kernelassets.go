// Package kernelassets embeds the Cubie A5E's GoSD Kconfig fragment so
// internal/kernelspec's Go-native KernelSpec (bean gosd-di6v) can be the
// single source of truth for kernel build inputs. go:embed can only reach
// files inside its own package directory, which is why this package lives
// alongside kernel-fragment.config rather than under internal/kernelspec
// itself. Unlike the Rockchip-family boards (radxa-zero-3e, nanopi-zero2,
// rock-4se), this board has no device-tree patches: header I2C/SPI
// enablement is deferred to a post-bring-up follow-up (locked in bean
// gosd-axtv — the dtsi has no SPI nodes at all at the pinned kernel tag, so
// there is nothing for an SPI patch to attach to yet), so there's no
// patches/ directory or PatchesFS to embed here.
package kernelassets

import _ "embed"

//go:embed kernel-fragment.config
var ConfigFragment []byte
