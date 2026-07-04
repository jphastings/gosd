// Package templates holds the Radxa Zero 3E's extlinux.conf as a go:embed
// text/template source, so the board profile that assembles a FAT boot
// partition can render it without shelling out or reading from disk.
//
// The content is locked by bean gosd-gbsz: it names the kernel, DTB, and
// initrd by the exact file names BootFiles writes into the boot partition,
// and the board ID GoSD boots with. Do not change it without updating that
// decision.
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
