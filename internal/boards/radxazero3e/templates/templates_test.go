package templates

import (
	"strings"
	"testing"
)

// Locked content, per bean gosd-gbsz: do not change this expectation without
// updating that decision.
const wantExtlinuxConf = "default gosd\n" +
	"timeout 0\n" +
	"label gosd\n" +
	"    kernel /Image\n" +
	"    fdt /rk3566-radxa-zero-3e.dtb\n" +
	"    initrd /initramfs.cpio.zst\n" +
	"    append console=ttyS2,1500000n8 quiet init=/init gosd.board=radxa-zero-3e\n"

func TestRenderExtlinuxConf(t *testing.T) {
	got, err := RenderExtlinuxConf(ExtlinuxConfData{ConsoleBaud: 1500000})
	if err != nil {
		t.Fatalf("RenderExtlinuxConf() error = %v", err)
	}
	if got != wantExtlinuxConf {
		t.Errorf("RenderExtlinuxConf() = %q, want %q", got, wantExtlinuxConf)
	}
}

func TestRenderExtlinuxConf_ConsoleBaudOverride(t *testing.T) {
	got, err := RenderExtlinuxConf(ExtlinuxConfData{ConsoleBaud: 115200})
	if err != nil {
		t.Fatalf("RenderExtlinuxConf() error = %v", err)
	}
	want := "    append console=ttyS2,115200n8 quiet init=/init gosd.board=radxa-zero-3e\n"
	if !strings.Contains(got, want) {
		t.Errorf("RenderExtlinuxConf(ConsoleBaud: 115200) = %q, want it to contain %q", got, want)
	}
}
