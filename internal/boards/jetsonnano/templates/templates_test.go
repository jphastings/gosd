package templates

import (
	"strings"
	"testing"
)

// Locked content, per bean gosd-cxio: do not change this expectation
// without updating that decision.
const wantExtlinuxConf = "default gosd\n" +
	"timeout 0\n" +
	"label gosd\n" +
	"    kernel /Image\n" +
	"    fdt /tegra210-p3450-0000.dtb\n" +
	"    initrd /initramfs.cpio.zst\n" +
	"    append console=ttyS0,115200n8 quiet init=/init gosd.board=jetson-nano panic=10\n"

func TestRenderExtlinuxConf(t *testing.T) {
	got, err := RenderExtlinuxConf(ExtlinuxConfData{ConsoleBaud: 115200})
	if err != nil {
		t.Fatalf("RenderExtlinuxConf() error = %v", err)
	}
	if got != wantExtlinuxConf {
		t.Errorf("RenderExtlinuxConf() = %q, want %q", got, wantExtlinuxConf)
	}
}

func TestRenderExtlinuxConf_KernelParams(t *testing.T) {
	got, err := RenderExtlinuxConf(ExtlinuxConfData{ConsoleBaud: 115200, KernelParams: "foo=bar baz"})
	if err != nil {
		t.Fatalf("RenderExtlinuxConf() error = %v", err)
	}
	want := "    append console=ttyS0,115200n8 quiet init=/init gosd.board=jetson-nano panic=10 foo=bar baz\n"
	if !strings.Contains(got, want) {
		t.Errorf("RenderExtlinuxConf(KernelParams: ...) = %q, want it to contain %q", got, want)
	}
}
