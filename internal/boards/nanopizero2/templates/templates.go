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
// it without updating that decision.
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

// RenderExtlinuxConf renders extlinux/extlinux.conf. Its content is fully
// locked (no per-build values), so it takes no data.
func RenderExtlinuxConf() (string, error) {
	var buf bytes.Buffer
	if err := extlinuxConf.Execute(&buf, nil); err != nil {
		return "", err
	}
	return buf.String(), nil
}
