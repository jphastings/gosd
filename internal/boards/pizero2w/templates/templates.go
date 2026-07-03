// Package templates holds the Pi Zero 2W boot partition text files
// (config.txt, cmdline.txt) as go:embed text/template sources, so the board
// profile that assembles a FAT boot partition can render them without
// shelling out or reading from disk.
//
// The content of both templates is locked by bean gosd-eu2x; do not change
// it without updating that decision.
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
