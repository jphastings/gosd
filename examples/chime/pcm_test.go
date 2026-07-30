package main

import (
	"strings"
	"testing"
)

func TestParseProcPCMKeepsPlaybackDevicesOnly(t *testing.T) {
	const procPCM = `00-00: bcm2835 Headphones : bcm2835 Headphones : playback 8
01-00: bcm2835 HDMI 1 : bcm2835 HDMI 1 : playback 8
02-00: USB Audio : USB Audio : playback 1 : capture 1
02-01: USB Audio MIC : USB Audio MIC : capture 1
`
	got := parseProcPCM(strings.NewReader(procPCM))
	if len(got) != 3 {
		t.Fatalf("parsed %d playback devices, want 3: %+v", len(got), got)
	}
	if got[1].card != 1 || got[1].device != 0 || got[1].name != "bcm2835 HDMI 1" {
		t.Errorf("second device = %+v, want card 1 device 0 named bcm2835 HDMI 1", got[1])
	}
	if got[1].path() != "/dev/snd/pcmC1D0p" {
		t.Errorf("path = %q, want /dev/snd/pcmC1D0p", got[1].path())
	}
	for _, d := range got {
		if d.name == "USB Audio MIC" {
			t.Error("a capture-only device was treated as a playback device")
		}
	}
}

func TestParseDevSndReadsCardAndDeviceFromNames(t *testing.T) {
	got := parseDevSnd([]string{
		"/dev/snd/pcmC0D0p",
		"/dev/snd/pcmC0D1c", // capture: not a playback node
		"/dev/snd/pcmC12D3p",
		"/dev/snd/controlC0", // not a PCM at all
	})
	if len(got) != 2 {
		t.Fatalf("parsed %d devices, want 2: %+v", len(got), got)
	}
	if got[1].card != 12 || got[1].device != 3 {
		t.Errorf("second device = card %d device %d, want card 12 device 3", got[1].card, got[1].device)
	}
}

func TestRankPrefersHDMIThenLowestCard(t *testing.T) {
	devices := []pcmDevice{
		{card: 0, device: 0, id: "bcm2835 Headphones", name: "bcm2835 Headphones"},
		{card: 3, device: 1, id: "bcm2835 IEC958/HDMI", name: "bcm2835 IEC958/HDMI"},
		{card: 2, device: 0, id: "vc4-hdmi", name: "MAI PCM i2s-hifi-0"},
		{card: 1, device: 0, id: "rockchip-es8316", name: "Analog"},
	}
	got := rank(devices)
	if !got[0].isHDMI() || !got[1].isHDMI() {
		t.Fatalf("HDMI devices should sort first, got %+v", got)
	}
	if got[0].card != 2 {
		t.Errorf("first choice is card %d, want the lowest-numbered HDMI card (2)", got[0].card)
	}
	if got[2].card != 0 || got[3].card != 1 {
		t.Errorf("non-HDMI devices should follow in card order, got cards %d then %d", got[2].card, got[3].card)
	}
}
