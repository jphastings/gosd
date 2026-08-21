// Package templates holds the Radxa ROCK 4SE's extlinux.conf as a go:embed
// text/template source, so the board profile that assembles a FAT boot
// partition can render it without shelling out or reading from disk.
//
// It names the kernel, DTB, and initrd by the exact file names BootFiles
// writes into the boot partition, and the board ID GoSD boots with. The
// console (ttyS2 @ 1500000n8, uart2) comes from bean gosd-je2r's research.
// ExtlinuxConfData.ConsoleBaud (gosd-zp9s) is an additive exception: it only
// ever changes the console= baud number, never the UART device (ttyS2) or
// anything else in the file. So is
// ExtlinuxConfData.KernelParams (gosd-mf3a): it only ever appends the
// developer's --kernel-param values to the end of the append line.
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
	// cmdline's console= argument, e.g. 1500000. See
	// boards.BuildConfig.ConsoleBaud / --console-baud.
	ConsoleBaud int

	// KernelParams is the developer's extra kernel command-line
	// parameters, already space-separated (see
	// boards.BuildConfig.KernelParamString / --kernel-param). Empty
	// renders the append line exactly as it was before the flag
	// existed; non-empty appends it, after everything gosd puts there
	// itself.
	KernelParams string
}

// RenderExtlinuxConf renders extlinux/extlinux.conf for the given data.
func RenderExtlinuxConf(data ExtlinuxConfData) (string, error) {
	var buf bytes.Buffer
	if err := extlinuxConf.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
