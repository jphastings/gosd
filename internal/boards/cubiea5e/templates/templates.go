// Package templates holds the Radxa Cubie A5E's extlinux.conf as a go:embed
// text/template source, so the board profile that assembles a FAT boot
// partition can render it without shelling out or reading from disk.
//
// It names the kernel, DTB, and initrd by the exact file names BootFiles
// writes into the boot partition, and the board ID GoSD boots with. The
// console (ttyS0 @ 115200n8, uart0) comes from bean gosd-jpc8's research -
// the board DT's stdout-path, a different UART and baud from the Rockchip
// boards' ttyS2 @ 1500000. ExtlinuxConfData.ConsoleBaud (gosd-zp9s) is an
// additive exception: it only ever changes the console= baud number, never
// the UART device (ttyS0) or anything else in the file.
package templates

import (
	"bytes"
	"embed"
	"text/template"
)

//go:embed extlinux.conf.tmpl
var extlinuxConfSrc string

// FS embeds the raw file too, so callers that want it for tooling (listing,
// hashing) can get at it without re-parsing.
//
//go:embed extlinux.conf.tmpl
var FS embed.FS

var extlinuxConf = template.Must(template.New("extlinux.conf").Parse(extlinuxConfSrc))

// ExtlinuxConfData holds the values interpolated into extlinux.conf.
type ExtlinuxConfData struct {
	// ConsoleBaud is the serial console baud rate baked into the kernel
	// cmdline's console= argument, e.g. 115200. See
	// boards.BuildConfig.ConsoleBaud / --console-baud.
	ConsoleBaud int

	// DTBFilename is the device tree blob extlinux loads, named rather
	// than hardcoded because this board ships two: the stock DTB, and a
	// variant with the ehci0/ohci0 host controllers disabled so the
	// USB-C port's phy stays with the peripheral controller. See
	// board.go's BootFiles and bean gosd-3io0 - the two are mutually
	// exclusive on this hardware.
	DTBFilename string
}

// RenderExtlinuxConf renders extlinux/extlinux.conf for the given data.
func RenderExtlinuxConf(data ExtlinuxConfData) (string, error) {
	var buf bytes.Buffer
	if err := extlinuxConf.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
