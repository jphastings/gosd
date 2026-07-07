package templates

import "testing"

// Locked content, per bean gosd-wskc: do not change this expectation without
// updating that decision.
const wantExtlinuxConf = "default gosd\n" +
	"timeout 0\n" +
	"label gosd\n" +
	"    kernel /Image\n" +
	"    fdt /rk3528-nanopi-zero2.dtb\n" +
	"    initrd /initramfs.cpio.zst\n" +
	"    append console=ttyS0,1500000n8 quiet init=/init gosd.board=nanopi-zero2\n"

func TestRenderExtlinuxConf(t *testing.T) {
	got, err := RenderExtlinuxConf()
	if err != nil {
		t.Fatalf("RenderExtlinuxConf() error = %v", err)
	}
	if got != wantExtlinuxConf {
		t.Errorf("RenderExtlinuxConf() = %q, want %q", got, wantExtlinuxConf)
	}
}
