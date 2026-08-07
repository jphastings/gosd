package templates

import (
	"strings"
	"testing"
)

// Locked content, per bean gosd-jpc8: do not change this expectation without
// updating that decision.
const wantExtlinuxConf = "default gosd\n" +
	"timeout 0\n" +
	"label gosd\n" +
	"    kernel /Image\n" +
	"    fdt /sun55i-a527-cubie-a5e.dtb\n" +
	"    initrd /initramfs.cpio.zst\n" +
	"    append console=ttyS0,115200n8 quiet init=/init gosd.board=cubie-a5e panic=10\n"

func TestRenderExtlinuxConf(t *testing.T) {
	got, err := RenderExtlinuxConf(ExtlinuxConfData{ConsoleBaud: 115200})
	if err != nil {
		t.Fatalf("RenderExtlinuxConf() error = %v", err)
	}
	if got != wantExtlinuxConf {
		t.Errorf("RenderExtlinuxConf() = %q, want %q", got, wantExtlinuxConf)
	}
}

func TestRenderExtlinuxConf_ConsoleBaudOverride(t *testing.T) {
	got, err := RenderExtlinuxConf(ExtlinuxConfData{ConsoleBaud: 1500000})
	if err != nil {
		t.Fatalf("RenderExtlinuxConf() error = %v", err)
	}
	want := "    append console=ttyS0,1500000n8 quiet init=/init gosd.board=cubie-a5e panic=10\n"
	if !strings.Contains(got, want) {
		t.Errorf("RenderExtlinuxConf(ConsoleBaud: 1500000) = %q, want it to contain %q", got, want)
	}
}
