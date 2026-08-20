package templates

import (
	"strings"
	"testing"
)

// Locked content, per beans gosd-06kj, gosd-85pt and gosd-fnza: do not change
// these expectations without updating that decision. The dtparam comment
// block is a gosd-dkqb addition: both dtparam lines are no-ops on this
// board specifically (see config.txt.tmpl), which the comment explains
// without removing the lines themselves.
const (
	wantConfigTxt = "kernel=kernel.img\n" +
		"initramfs initramfs.cpio.zst followkernel\n" +
		"enable_uart=1\n" +
		"disable_splash=1\n" +
		"boot_delay=0\n" +
		"avoid_warnings=1\n" +
		"# GoSD (github.com/jphastings/gosd): dtparam=i2c_arm=on and dtparam=spi=on\n" +
		"# are both no-ops on THIS board - dtparam works by the Pi firmware patching\n" +
		"# the DTB's __overrides__ block at boot, and pi-zero-w is the one GoSD\n" +
		"# board built from the mainline-style DTS chain, which carries no\n" +
		"# __overrides__ node at all (bean gosd-dkqb). They're kept here anyway, for\n" +
		"# the same reason every other Pi board's config.txt carries them: as a\n" +
		"# statement of intent, and because the firmware only logs and ignores an\n" +
		"# unrecognised dtparam rather than failing the boot. I2C is unaffected -\n" +
		"# bcm2835-rpi.dtsi sets &i2c0/&i2c1 status = \"okay\" unconditionally, so it\n" +
		"# works regardless of this line. SPI is NOT unaffected: it needs (and, in\n" +
		"# tree, now has) a kernel-build DTS patch instead -\n" +
		"# build/boards/pi-zero-w/kernel/patches/0003-enable-header-spi.patch sets\n" +
		"# &spi status = \"okay\" directly, since dtparam can't reach this DTB.\n" +
		"dtparam=i2c_arm=on\n" +
		"dtparam=spi=on\n"

	wantCmdlineTxt = "console=serial0,115200 quiet init=/init gosd.board=pi-zero-w panic=10"
)

func TestRenderConfigTxt(t *testing.T) {
	got, err := RenderConfigTxt(ConfigTxtData{InitramfsName: "initramfs.cpio.zst"})
	if err != nil {
		t.Fatalf("RenderConfigTxt() error = %v", err)
	}
	if got != wantConfigTxt {
		t.Errorf("RenderConfigTxt() = %q, want %q", got, wantConfigTxt)
	}
}

func TestRenderConfigTxt_NoArm64bitLine(t *testing.T) {
	got, err := RenderConfigTxt(ConfigTxtData{InitramfsName: "initramfs.cpio.zst"})
	if err != nil {
		t.Fatalf("RenderConfigTxt() error = %v", err)
	}
	if strings.Contains(got, "arm_64bit") {
		t.Errorf("RenderConfigTxt() = %q, want no arm_64bit line (pi-zero-w is 32-bit only)", got)
	}
}

func TestRenderCmdlineTxt(t *testing.T) {
	got, err := RenderCmdlineTxt(CmdlineTxtData{Board: "pi-zero-w", ConsoleBaud: 115200})
	if err != nil {
		t.Fatalf("RenderCmdlineTxt() error = %v", err)
	}
	if got != wantCmdlineTxt {
		t.Errorf("RenderCmdlineTxt() = %q, want %q", got, wantCmdlineTxt)
	}
}

func TestRenderCmdlineTxt_IsSingleLine(t *testing.T) {
	got, err := RenderCmdlineTxt(CmdlineTxtData{Board: "pi-zero-w", ConsoleBaud: 115200})
	if err != nil {
		t.Fatalf("RenderCmdlineTxt() error = %v", err)
	}
	for i, r := range got {
		if r == '\n' {
			t.Fatalf("RenderCmdlineTxt() contains a newline at byte %d, want a single line: %q", i, got)
		}
	}
}

func TestRenderCmdlineTxt_ConsoleBaudOverride(t *testing.T) {
	got, err := RenderCmdlineTxt(CmdlineTxtData{Board: "pi-zero-w", ConsoleBaud: 57600})
	if err != nil {
		t.Fatalf("RenderCmdlineTxt() error = %v", err)
	}
	want := "console=serial0,57600 quiet init=/init gosd.board=pi-zero-w panic=10"
	if got != want {
		t.Errorf("RenderCmdlineTxt(ConsoleBaud: 57600) = %q, want %q", got, want)
	}
}

func TestRenderConfigTxt_InterpolatesInitramfsName(t *testing.T) {
	got, err := RenderConfigTxt(ConfigTxtData{InitramfsName: "custom-initramfs.cpio.zst"})
	if err != nil {
		t.Fatalf("RenderConfigTxt() error = %v", err)
	}
	want := "initramfs custom-initramfs.cpio.zst followkernel\n"
	if !strings.Contains(got, want) {
		t.Errorf("RenderConfigTxt() = %q, want it to contain %q", got, want)
	}
}

func TestRenderConfigTxt_I2cEnabledByDefault(t *testing.T) {
	got, err := RenderConfigTxt(ConfigTxtData{InitramfsName: "initramfs.cpio.zst"})
	if err != nil {
		t.Fatalf("RenderConfigTxt() error = %v", err)
	}
	if !strings.Contains(got, "dtparam=i2c_arm=on\n") {
		t.Errorf("RenderConfigTxt() = %q, want it to contain dtparam=i2c_arm=on (I2C is enabled by default, no opt-out flag)", got)
	}
}

func TestRenderConfigTxt_SpiEnabledByDefault(t *testing.T) {
	got, err := RenderConfigTxt(ConfigTxtData{InitramfsName: "initramfs.cpio.zst"})
	if err != nil {
		t.Fatalf("RenderConfigTxt() error = %v", err)
	}
	if !strings.Contains(got, "dtparam=spi=on\n") {
		t.Errorf("RenderConfigTxt() = %q, want it to contain dtparam=spi=on (SPI is enabled by default, no opt-out flag)", got)
	}
}

func TestRenderConfigTxt_UsbGadgetAddsDwc2Overlay(t *testing.T) {
	got, err := RenderConfigTxt(ConfigTxtData{InitramfsName: "initramfs.cpio.zst", UsbGadget: true})
	if err != nil {
		t.Fatalf("RenderConfigTxt() error = %v", err)
	}
	want := wantConfigTxt + "dtoverlay=dwc2,dr_mode=peripheral\n"
	if got != want {
		t.Errorf("RenderConfigTxt() = %q, want %q", got, want)
	}
}
