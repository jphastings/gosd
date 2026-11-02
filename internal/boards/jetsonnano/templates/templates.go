// Package templates holds the Jetson Nano's extlinux.conf as a go:embed
// text/template source, so the board profile that assembles a FAT boot
// partition can render it without shelling out or reading from disk.
//
// The content is locked by bean gosd-cxio: it names the kernel, DTB, and
// initrd by the exact file names BootFiles writes into the boot partition,
// and the board ID GoSD boots with. Do not change it without updating that
// decision. `fdt` (not `fdtdir`) is deliberate: NVIDIA's U-Boot skips a
// label whose named FDT is missing, whereas a missing `fdtdir` blob falls
// back to CBoot's L4T DTB — the pairing the epic (gosd-72je) forbids.
// ExtlinuxConfData.ConsoleBaud only ever changes the console= baud number,
// never the UART device (ttyS0: tegra210-p3450-0000.dts aliases serial0 to
// UART-A) or anything else in the file. ExtlinuxConfData.KernelParams only
// ever appends the developer's --kernel-param values to the end of the
// append line.
package templates

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed extlinux.conf.tmpl
var extlinuxConfSrc string

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
	// renders nothing extra; non-empty appends it, after everything gosd
	// puts there itself.
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
