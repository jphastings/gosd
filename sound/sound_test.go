package sound

import (
	"errors"
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

// The four outputs a GoSD board can plausibly show at once, deliberately out of
// card order so ranking has something to do.
func boardPCMs() []pcm {
	return []pcm{
		{card: 0, device: 0, id: "bcm2835 Headphones", name: "bcm2835 Headphones"},
		{card: 3, device: 1, id: "bcm2835 IEC958/HDMI", name: "bcm2835 IEC958/HDMI"},
		{card: 2, device: 0, id: "hdmi-sound", name: "hdmi-sound"},
		{card: 1, device: 0, id: "rockchip-es8316", name: "Analog"},
	}
}

func TestRankHonoursThePreferredOutput(t *testing.T) {
	for _, tc := range []struct {
		prefer   Output
		wantCard int
		wantHDMI bool
	}{
		{prefer: HDMI, wantCard: 2, wantHDMI: true},
		{prefer: Analog, wantCard: 0, wantHDMI: false},
		{prefer: Any, wantCard: 0, wantHDMI: false}, // no preference: card order
	} {
		got := rank(boardPCMs(), tc.prefer)
		if got[0].card != tc.wantCard || got[0].isHDMI() != tc.wantHDMI {
			t.Errorf("prefer %v chose card %d (HDMI %t), want card %d (HDMI %t)",
				tc.prefer, got[0].card, got[0].isHDMI(), tc.wantCard, tc.wantHDMI)
		}
		if len(got) != 4 {
			t.Errorf("prefer %v returned %d devices, want all 4", tc.prefer, len(got))
		}
	}
}

func TestRankKeepsCardOrderWithinAPreference(t *testing.T) {
	got := rank(boardPCMs(), HDMI)
	if !got[0].isHDMI() || !got[1].isHDMI() {
		t.Fatalf("both HDMI devices should sort first, got %+v", got)
	}
	if got[0].card != 2 || got[1].card != 3 {
		t.Errorf("HDMI devices came in card order %d, %d; want 2, 3", got[0].card, got[1].card)
	}
	if got[2].card != 0 || got[3].card != 1 {
		t.Errorf("analog devices came in card order %d, %d; want 0, 1", got[2].card, got[3].card)
	}
}

func TestParsePathRejectsAnythingButAPlaybackPCM(t *testing.T) {
	got, err := parsePath("/dev/snd/pcmC1D2p")
	if err != nil {
		t.Fatalf("parsing a valid path: %v", err)
	}
	if got.card != 1 || got.device != 2 {
		t.Errorf("parsed card %d device %d, want card 1 device 2", got.card, got.device)
	}
	for _, bad := range []string{
		"/dev/snd/pcmC1D2c", // capture
		"/dev/snd/controlC1",
		"/dev/snd/pcmCxD2p",
		"hw:1,2",
		"",
	} {
		if _, err := parsePath(bad); err == nil {
			t.Errorf("parsePath(%q) succeeded, want an error naming the expected shape", bad)
		} else if !strings.Contains(err.Error(), "pcmC0D0p") {
			t.Errorf("parsePath(%q) error %q does not show the expected path shape", bad, err)
		}
	}
}

// The zero-value Options must ask for the two rates every GoSD audio path
// supports, in the order that gets HDMI's native rate first.
func TestOptionsFormatsDefaultsToStereoAt48Then44Point1(t *testing.T) {
	got := Options{}.formats()
	want := []Format{{Rate: 48000, Channels: 2}, {Rate: 44100, Channels: 2}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("default formats = %v, want %v", got, want)
	}
	if only := (Options{Format: Format{Rate: 8000, Channels: 1}}).formats(); len(only) != 1 || only[0].Rate != 8000 || only[0].Channels != 1 {
		t.Errorf("an explicit format gave %v, want only 8000 Hz mono", only)
	}
	if mono := (Options{Format: Format{Channels: 1}}).formats(); len(mono) != 2 || mono[0].Channels != 1 || mono[1].Channels != 1 {
		t.Errorf("channels-only options gave %v, want both default rates in mono", mono)
	}
}

func TestFrameBytesCountsEverySample(t *testing.T) {
	if got := (Format{Rate: 48000, Channels: 2}).FrameBytes(); got != 4 {
		t.Errorf("stereo frame = %d bytes, want 4", got)
	}
	if got := (Format{Rate: 48000, Channels: 1}).FrameBytes(); got != 2 {
		t.Errorf("mono frame = %d bytes, want 2", got)
	}
}

// The no-device error is the one message most users of this package will ever
// see, so it has to distinguish "no sound in this kernel" from "sound, but no
// card" and name the fix for the first.
func TestNoDeviceErrorNamesTheFix(t *testing.T) {
	noSnd := noDeviceError(false)
	if !errors.Is(noSnd, ErrNoDevice) {
		t.Error("a missing /dev/snd should wrap ErrNoDevice")
	}
	for _, want := range []string{"/dev/snd", "gosd build-kernel", "docs/sound.md"} {
		if !strings.Contains(noSnd.Error(), want) {
			t.Errorf("error %q does not mention %q", noSnd, want)
		}
	}

	noCard := noDeviceError(true)
	if !errors.Is(noCard, ErrNoDevice) {
		t.Error("a soundless-but-present /dev/snd should wrap ErrNoDevice")
	}
	if !strings.Contains(noCard.Error(), "before power-up") {
		t.Errorf("error %q does not mention the HDMI-at-boot requirement", noCard)
	}
	if strings.Contains(noCard.Error(), "build-kernel") {
		t.Errorf("error %q tells the user to rebuild a kernel that already has sound", noCard)
	}
}
