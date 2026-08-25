// Package kernelassets embeds the Turing RK1's GoSD Kconfig fragment so
// internal/kernelspec's Go-native KernelSpec (bean gosd-di6v) can be the
// single source of truth for kernel build inputs. go:embed can only reach
// files inside its own package directory, which is why this package lives
// alongside kernel-fragment.config rather than under internal/kernelspec
// itself.
//
// Unlike the rest of the Rockchip fleet, this board has no device-tree
// patches at all (like qemu-virt, the only other unpatched board in the
// fleet): its epic scopes GPIO/peripheral header enablement out entirely,
// and rk3588-turing-rk1.dtsi has no `led`/`gpio-leds` node for the
// fleet-wide retain-state-on-shutdown patch (gosd-54j8) to attach to
// either — confirmed by reading the file directly, not assumed from its
// absence in a grep.
package kernelassets

import _ "embed"

//go:embed kernel-fragment.config
var ConfigFragment []byte
