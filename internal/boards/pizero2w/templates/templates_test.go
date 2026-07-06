package templates

import (
	"strings"
	"testing"
)

// Locked content, per bean gosd-eu2x: do not change these expectations
// without updating that decision.
const (
	wantConfigTxt = "arm_64bit=1\n" +
		"kernel=kernel8.img\n" +
		"initramfs initramfs.cpio.zst followkernel\n" +
		"enable_uart=1\n" +
		"disable_splash=1\n" +
		"boot_delay=0\n" +
		"avoid_warnings=1\n"

	wantCmdlineTxt = "console=serial0,115200 quiet init=/init gosd.board=pi-zero-2w"
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

func TestRenderCmdlineTxt(t *testing.T) {
	got, err := RenderCmdlineTxt(CmdlineTxtData{Board: "pi-zero-2w"})
	if err != nil {
		t.Fatalf("RenderCmdlineTxt() error = %v", err)
	}
	if got != wantCmdlineTxt {
		t.Errorf("RenderCmdlineTxt() = %q, want %q", got, wantCmdlineTxt)
	}
}

func TestRenderCmdlineTxt_IsSingleLine(t *testing.T) {
	got, err := RenderCmdlineTxt(CmdlineTxtData{Board: "pi-zero-2w"})
	if err != nil {
		t.Fatalf("RenderCmdlineTxt() error = %v", err)
	}
	for i, r := range got {
		if r == '\n' {
			t.Fatalf("RenderCmdlineTxt() contains a newline at byte %d, want a single line: %q", i, got)
		}
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
