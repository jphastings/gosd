// Package templates holds the Radxa ROCK 4SE's extlinux.conf as a go:embed
// text/template source, so the board profile that assembles a FAT boot
// partition can render it without shelling out or reading from disk.
//
// It names the kernel, DTB, and initrd by the exact file names BootFiles
// writes into the boot partition, and the board ID GoSD boots with. The
// console (ttyS2 @ 1500000n8, uart2) comes from bean gosd-je2r's research.
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

// RenderExtlinuxConf renders extlinux/extlinux.conf. Its content is fully
// locked (no per-build values), so it takes no data.
func RenderExtlinuxConf() (string, error) {
	var buf bytes.Buffer
	if err := extlinuxConf.Execute(&buf, nil); err != nil {
		return "", err
	}
	return buf.String(), nil
}
