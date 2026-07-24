// Package templates holds the Radxa Zero 3E's extlinux.conf as a go:embed
// text/template source, so the board profile that assembles a FAT boot
// partition can render it without shelling out or reading from disk.
//
// The content is locked by bean gosd-gbsz: it names the kernel, DTB, and
// initrd by the exact file names BootFiles writes into the boot partition,
// and the board ID GoSD boots with. Do not change it without updating that
// decision. ConsoleBaudData.ConsoleBaud (gosd-zp9s) is an additive exception:
// it only ever changes the console= baud number, never the UART device
// (ttyS2) or anything else in the file.
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
