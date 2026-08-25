// Package templates holds the Turing RK1's extlinux.conf as a go:embed
// text/template source, so the board profile that assembles a FAT boot
// partition can render it without shelling out or reading from disk.
//
// The content is locked by bean gosd-jvtg: it names the kernel, DTB, and
// initrd by the exact file names BootFiles writes into the boot partition,
// and the board ID GoSD boots with. Do not change it without updating that
// decision. ConsoleBaudData.ConsoleBaud is an additive exception: it only
// ever changes the console= baud number, never the UART device (ttyS9,
// from bean gosd-k4w2's research against rk3588-turing-rk1.dts's
// stdout-path — the serial9 DT alias, not yet hardware-confirmed to map to
// ttyS9) or anything else in the file. So is ExtlinuxConfData.KernelParams:
// it only ever appends the developer's --kernel-param values to the end of
// the append line.
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
	// cmdline's console= argument, e.g. 115200. See
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
