// Package kernelassets embeds the Jetson Nano's GoSD Kconfig fragment and
// device-tree patches so internal/kernelspec's Go-native KernelSpec (bean
// gosd-di6v) can be the single source of truth for kernel build inputs. The
// embed directive can only reach files inside its own package directory,
// which is why this package lives alongside kernel-fragment.config and
// patches/ rather than under internal/kernelspec itself.
//
// Mainline only describes the SD-card dev kit (tegra210-p3450-0000.dts), so
// the one patch adapts that DTB to the eMMC module GoSD targets: it enables
// the eMMC on SDMMC4, pins it to mmc0, and disables the dev kit's SD slot
// and M.2 Key E SDIO (bean gosd-wxy7). There's no status-LED patch: the DTS
// has no gpio-leds node for the fleet-wide retain-state-on-shutdown patch
// (gosd-54j8) to attach to. Header I2C/SPI enablement is a follow-up (see
// docs/development/jetson-nano.md).
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
