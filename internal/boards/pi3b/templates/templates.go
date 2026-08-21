// Package templates holds the Pi 3B boot partition text files (config.txt,
// cmdline.txt) as go:embed text/template sources, so the board profile that
// assembles a FAT boot partition can render them without shelling out or
// reading from disk.
//
// The content of both templates is locked by bean gosd-ypg1 (epic
// gosd-xhc3); do not change it without updating that decision. Unlike the
// two Pi Zero boards' templates there is no UsbGadget branch: the 3B's SoC
// USB port is hard-wired through the onboard LAN9514 hub, so it can never be
// put into peripheral mode — a dwc2 overlay here would only break the
// board's Ethernet. CmdlineTxtData.ConsoleBaud (gosd-zp9s) is an additive
// exception: it only ever changes the console= baud number, never the UART
// device (serial0 — the mini-UART on this BT-equipped board) or anything
// else on the line. So is CmdlineTxtData.KernelParams (gosd-mf3a):
// it only ever appends the developer's --kernel-param values after the
// locked ones.
package templates

import (
	"bytes"
	"embed"
	"text/template"
)

//go:embed config.txt.tmpl
var configTxtSrc string

//go:embed cmdline.txt.tmpl
var cmdlineTxtSrc string

// Also embed the raw files so callers that want the templates.FS for
// tooling (e.g. listing, hashing) can get at them without re-parsing.
//
//go:embed config.txt.tmpl cmdline.txt.tmpl
var FS embed.FS

var (
	configTxt  = template.Must(template.New("config.txt").Parse(configTxtSrc))
	cmdlineTxt = template.Must(template.New("cmdline.txt").Parse(cmdlineTxtSrc))
)

// ConfigTxtData holds the values interpolated into config.txt.
type ConfigTxtData struct {
	// InitramfsName is the initramfs file name on the FAT boot partition,
	// e.g. "initramfs.cpio.zst".
	InitramfsName string
}

// CmdlineTxtData holds the values interpolated into cmdline.txt.
type CmdlineTxtData struct {
	// Board is the gosd board ID, passed through as gosd.board=<Board>.
	Board string
	// ConsoleBaud is the serial console baud rate baked into console=,
	// e.g. 115200. See boards.BuildConfig.ConsoleBaud / --console-baud.
	ConsoleBaud int
	// KernelParams is the developer's extra kernel command-line
	// parameters, already space-separated (see
	// boards.BuildConfig.KernelParamString / --kernel-param). Empty
	// renders the line exactly as it was before the flag existed;
	// non-empty appends it, after everything gosd puts there itself.
	KernelParams string
}

// RenderConfigTxt renders config.txt for the given data.
func RenderConfigTxt(data ConfigTxtData) (string, error) {
	var buf bytes.Buffer
	if err := configTxt.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderCmdlineTxt renders cmdline.txt for the given data.
func RenderCmdlineTxt(data CmdlineTxtData) (string, error) {
	var buf bytes.Buffer
	if err := cmdlineTxt.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
