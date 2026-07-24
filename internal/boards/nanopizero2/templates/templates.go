// Package templates holds the NanoPi Zero2's extlinux.conf as a go:embed
// text/template source, so the board profile that assembles a FAT boot
// partition can render it without shelling out or reading from disk.
//
// The content is locked by bean gosd-wskc: it names the kernel, DTB, and
// initrd by the exact file names BootFiles writes into the boot partition,
// the board ID GoSD boots with, and the debug UART (UART0, ttyS0,
// 1500000n8 — verified against the mainline rk3528-nanopi-zero2.dts aliases
// node and rk3528.dtsi's uart0 node at kernel tag v6.18.37: serial0 = &uart0,
// uart0 is "rockchip,rk3528-uart", "snps,dw-apb-uart" — the standard 8250
// family driver, so it enumerates as /dev/ttyS0, not ttyFIQ). Do not change
// it without updating that decision. ExtlinuxConfData.ConsoleBaud
// (gosd-zp9s) is an additive exception: it only ever changes the console=
// baud number, never the UART device (ttyS0) or anything else in the file.
package templates

import (
	"bytes"
	"embed"
	"text/template"
)

//go:embed extlinux.conf.tmpl
var extlinuxConfSrc string

// Also embed the raw file so callers that want the templates.FS for tooling
// (e.g. listing, hashing) can get at it without re-parsing.
//
//go:embed extlinux.conf.tmpl
var FS embed.FS

var extlinuxConf = template.Must(template.New("extlinux.conf").Parse(extlinuxConfSrc))

// ExtlinuxConfData holds the values interpolated into extlinux.conf.
type ExtlinuxConfData struct {
	// ConsoleBaud is the serial console baud rate baked into the kernel
	// cmdline's console= argument, e.g. 1500000. See
	// boards.BuildConfig.ConsoleBaud / --console-baud.
	ConsoleBaud int
}

// RenderExtlinuxConf renders extlinux/extlinux.conf for the given data.
func RenderExtlinuxConf(data ExtlinuxConfData) (string, error) {
	var buf bytes.Buffer
	if err := extlinuxConf.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
