// Package kernelassets embeds the Jetson Nano's GoSD Kconfig fragment so
// internal/kernelspec's Go-native KernelSpec (bean gosd-di6v) can be the
// single source of truth for kernel build inputs. go:embed can only reach
// files inside its own package directory, which is why this package lives
// alongside kernel-fragment.config rather than under internal/kernelspec
// itself.
//
// There are no device-tree patches: tegra210-p3450-0000.dts has no
// gpio-leds node for the fleet-wide retain-state-on-shutdown patch
// (gosd-54j8) to attach to (the dev kit's only LED is hard-wired power), and
// header I2C/SPI enablement is a follow-up (see
// docs/development/jetson-nano.md).
package kernelassets

import _ "embed"

//go:embed kernel-fragment.config
var ConfigFragment []byte
