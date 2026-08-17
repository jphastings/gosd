// Package kernelassets embeds the Cubie A5E's GoSD Kconfig fragment and
// device-tree patches so internal/kernelspec's Go-native KernelSpec (bean
// gosd-di6v) can be the single source of truth for kernel build inputs.
// Embedding can only reach files inside its own package directory, which
// is why this package lives alongside kernel-fragment.config rather than
// under internal/kernelspec itself.
//
// Unlike the Rockchip-family boards, whose patches enable header
// peripherals, this board's one patch adds a second, USB-gadget variant of
// the board DTB (bean gosd-3io0): its USB-C port is the only OTG-capable
// one, its ehci0/ohci0 host controllers share usbphy port 0 with usb_otg,
// and it has no ID/VBUS detection to arbitrate between them — so host and
// peripheral are mutually exclusive and the choice has to be made when the
// image is built. Header I2C/SPI enablement remains deferred to a
// post-bring-up follow-up (locked in bean gosd-axtv — the dtsi has no SPI
// nodes at all at the pinned kernel tag, so there is nothing for an SPI
// patch to attach to yet).
package kernelassets

import "embed"

//go:embed kernel-fragment.config
var ConfigFragment []byte

// PatchesFS embeds every device-tree patch applied (in filename order,
// `patch -p1`) before the config step — see internal/kernelbuild's DTS-patch
// application step.
//
//go:embed patches
var PatchesFS embed.FS
