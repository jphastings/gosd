package sound

import (
	"strings"
	"testing"
)

// The ES8316 on a ROCK 4SE's headphone jack, as its driver really names its
// controls (sound/soc/codecs/es8316.c at the pinned kernel), with the values a
// freshly powered-up codec really holds. This is the card that played perfect
// silence on the bench: the PCM accepted every frame and the headphone mixer's
// DAC inputs were switched off the whole time.
func es8316Controls() []Control {
	boolean := func(numid int, name string, on int) Control {
		return Control{Numid: numid, Iface: IfaceMixer, Name: name, Type: ControlBoolean,
			Min: 0, Max: 1, Step: 1, Values: []int{on}, Readable: true, Writable: true}
	}
	integer := func(numid int, name string, minVal, maxVal int, values ...int) Control {
		return Control{Numid: numid, Iface: IfaceMixer, Name: name, Type: ControlInteger,
			Min: minVal, Max: maxVal, Step: 1, Values: values, Readable: true, Writable: true}
	}
	enum := func(numid int, name string, at int, items ...string) Control {
		return Control{Numid: numid, Iface: IfaceMixer, Name: name, Type: ControlEnumerated,
			Max: len(items) - 1, Items: items, Values: []int{at}, Readable: true, Writable: true}
	}
	analogIn := []string{"lin1-rin1", "lin2-rin2", "lin-rin with Boost", "lin-rin with Boost and PGA"}
	return []Control{
		integer(1, "Headphone Playback Volume", 0, 0, 0, 0),
		integer(2, "Headphone Mixer Volume", 0, 11, 0, 0),
		enum(3, "Playback Polarity", 0, "Normal", "R Invert", "L Invert", "L + R Invert"),
		integer(4, "DAC Playback Volume", 0, 192, 192, 192),
		boolean(5, "DAC Soft Ramp Switch", 0),
		boolean(6, "DAC Notch Filter Switch", 0),
		boolean(7, "DAC Double Fs Switch", 0),
		boolean(8, "DAC Mono Mix Switch", 0),
		boolean(9, "Mic Boost Switch", 0),
		integer(10, "ADC Capture Volume", 0, 192, 0),
		boolean(11, "ALC Capture Switch", 0),
		boolean(12, "Left Headphone Mixer LLIN Switch", 0),
		boolean(13, "Left Headphone Mixer Left DAC Switch", 0),
		boolean(14, "Right Headphone Mixer RLIN Switch", 0),
		boolean(15, "Right Headphone Mixer Right DAC Switch", 0),
		enum(16, "Left Headphone Mux", 0, analogIn...),
		enum(17, "Right Headphone Mux", 0, analogIn...),
		enum(18, "DAC Source Mux", 0, "LDATA TO LDAC, RDATA TO RDAC", "LDATA TO LDAC, LDATA TO RDAC"),
		enum(19, "Differential Mux", 0, analogIn...),
	}
}

func changesByName(changes []Change) map[string][]int {
	out := make(map[string][]int, len(changes))
	for _, c := range changes {
		out[c.Name] = c.To
	}
	return out
}

// The whole point of the pass: a codec that powers up with its output mixer
// disconnected must end up connected, without the pass wandering into the
// codec's DSP features or its input path on the way.
func TestAudibilityPassConnectsTheDACToTheHeadphones(t *testing.T) {
	got := changesByName(audibilityPass(es8316Controls(), DefaultVolume, Any))

	for _, name := range []string{
		"Left Headphone Mixer Left DAC Switch",
		"Right Headphone Mixer Right DAC Switch",
	} {
		if to, ok := got[name]; !ok || to[0] != 1 {
			t.Errorf("%q was set to %v, want it switched on: without it the DAC never reaches the jack", name, to)
		}
	}
	if to, ok := got["Headphone Mixer Volume"]; !ok || to[0] != 8 || to[1] != 8 {
		t.Errorf("Headphone Mixer Volume was set to %v, want 8,8 (75%% of 0..11)", to)
	}
	if len(got) != 3 {
		t.Errorf("the pass made %d changes, want exactly 3: %v", len(got), got)
	}
}

// Each of these is a control the pass must not touch, and a different reason
// why. They are all real ES8316 control names.
func TestAudibilityPassLeavesEverythingElseAlone(t *testing.T) {
	got := changesByName(audibilityPass(es8316Controls(), DefaultVolume, Any))
	for name, why := range map[string]string{
		"DAC Notch Filter Switch":          "a DSP feature, not a mute",
		"DAC Double Fs Switch":             "a DSP feature, not a mute",
		"DAC Mono Mix Switch":              "would fold stereo to mono",
		"DAC Soft Ramp Switch":             "a DSP feature, not a mute",
		"Left Headphone Mixer LLIN Switch": "would fold the line/mic input into the headphones",
		"Mic Boost Switch":                 "input path",
		"ALC Capture Switch":               "input path",
		"ADC Capture Volume":               "input path",
		"Left Headphone Mux":               "an analog input mux with no DAC choice",
		"Differential Mux":                 "an input mux",
		"Playback Polarity":                "an enum with no DAC choice",
		"DAC Source Mux":                   "its choices name LDAC/RDAC, not a DAC source",
		"DAC Playback Volume":              "already at 0 dB; the pass never turns anything down",
		"Headphone Playback Volume":        "a single-valued control with nothing to set",
	} {
		if to, touched := got[name]; touched {
			t.Errorf("the pass set %q to %v; it must not: %s", name, to, why)
		}
	}
}

// snd_bcm2835's two controls. Its volume is in hundredths of a dB over a
// -102.39..+4 dB range and powers up at 0 dB, so a naive "set it to 75% of the
// range" would quietly attenuate the Pi by 22 dB.
func piControls(volumeAt int) []Control {
	return []Control{
		{Numid: 1, Iface: IfaceMixer, Name: "PCM Playback Volume", Type: ControlInteger,
			Min: -10239, Max: 400, Step: 1, Values: []int{volumeAt}, Readable: true, Writable: true},
		{Numid: 2, Iface: IfaceMixer, Name: "PCM Playback Switch", Type: ControlBoolean,
			Min: 0, Max: 1, Step: 1, Values: []int{0}, Readable: true, Writable: true},
	}
}

func TestAudibilityPassNeverTurnsAControlDown(t *testing.T) {
	atZeroDB := changesByName(audibilityPass(piControls(0), DefaultVolume, Any))
	if to, touched := atZeroDB["PCM Playback Volume"]; touched {
		t.Errorf("the pass moved a volume that was already at 0 dB to %v", to)
	}
	if to := atZeroDB["PCM Playback Switch"]; len(to) != 1 || to[0] != 1 {
		t.Errorf("PCM Playback Switch was set to %v, want it unmuted", to)
	}

	muted := changesByName(audibilityPass(piControls(-10239), DefaultVolume, Any))
	if to, ok := muted["PCM Playback Volume"]; !ok || to[0] != -2260 {
		t.Errorf("a volume at the bottom of its range was set to %v, want -2260 (75%% of -10239..400)", to)
	}
}

func TestAudibilityPassVolumeIsAPercentageOfRange(t *testing.T) {
	for _, tc := range []struct{ volume, want int }{
		{volume: 100, want: 400},
		{volume: 75, want: -2260},
		{volume: 1, want: -10133},
	} {
		got := changesByName(audibilityPass(piControls(-10239), tc.volume, Any))
		if to, ok := got["PCM Playback Volume"]; !ok || to[0] != tc.want {
			t.Errorf("volume %d%% set the control to %v, want %d", tc.volume, to, tc.want)
		}
	}
}

// A card whose outputs share one PCM behind a routing control is the only case
// where Options.Prefer has anything to set — on the Pi, HDMI and the jack are
// separate cards, so Prefer picks between PCMs instead.
func routedCard() []Control {
	return []Control{
		{Numid: 1, Iface: IfaceMixer, Name: "PCM Playback Route", Type: ControlEnumerated,
			Max: 2, Items: []string{"Auto", "Headphones", "HDMI"}, Values: []int{0},
			Readable: true, Writable: true},
	}
}

func TestPreferSetsAnEnumeratedRouteControl(t *testing.T) {
	for _, tc := range []struct {
		prefer Output
		want   int
		set    bool
	}{
		{prefer: HDMI, want: 2, set: true},
		{prefer: Analog, want: 1, set: true},
		{prefer: Any, set: false},
	} {
		got := changesByName(audibilityPass(routedCard(), DefaultVolume, tc.prefer))
		to, ok := got["PCM Playback Route"]
		if ok != tc.set {
			t.Errorf("prefer %v: route control touched = %t, want %t", tc.prefer, ok, tc.set)
			continue
		}
		if tc.set && to[0] != tc.want {
			t.Errorf("prefer %v routed to %v, want item %d", tc.prefer, to, tc.want)
		}
	}
}

// An integer route control's values mean whatever its driver says they mean,
// so the pass leaves it where the driver put it rather than guessing.
func TestPreferIgnoresANonEnumeratedRouteControl(t *testing.T) {
	elements := []Control{
		{Numid: 1, Iface: IfaceMixer, Name: "PCM Playback Route", Type: ControlInteger,
			Min: 0, Max: 2, Step: 1, Values: []int{0}, Readable: true, Writable: true},
	}
	if got := audibilityPass(elements, DefaultVolume, HDMI); len(got) != 0 {
		t.Errorf("the pass changed %v; an integer route control's values are driver-specific", got)
	}
}

// An output stage that really does select its source with a mux is the case
// Options.Prefer cannot help with and the pass can.
func TestAudibilityPassPointsAnOutputMuxAtTheDAC(t *testing.T) {
	elements := []Control{
		{Numid: 1, Iface: IfaceMixer, Name: "Speaker Mux", Type: ControlEnumerated,
			Max: 2, Items: []string{"Line In", "DAC", "Sidetone"}, Values: []int{0},
			Readable: true, Writable: true},
	}
	got := changesByName(audibilityPass(elements, DefaultVolume, Any))
	if to, ok := got["Speaker Mux"]; !ok || to[0] != 1 {
		t.Fatalf("Speaker Mux was set to %v, want the DAC choice (item 1)", to)
	}
	elements[0].Values = []int{1}
	if got := audibilityPass(elements, DefaultVolume, Any); len(got) != 0 {
		t.Errorf("a mux already on the DAC was rewritten: %v", got)
	}
}

func TestAudibilityPassSkipsWhatItCannotOrMustNotWrite(t *testing.T) {
	elements := []Control{
		{Numid: 1, Iface: IfaceMixer, Name: "Master Playback Switch", Type: ControlBoolean,
			Max: 1, Values: []int{0}, Readable: true, Writable: false},
		{Numid: 2, Iface: IfacePCM, Name: "IEC958 Playback Switch", Type: ControlBoolean,
			Max: 1, Values: []int{0}, Readable: true, Writable: true},
		{Numid: 3, Iface: IfaceMixer, Name: "Master Playback Volume", Type: ControlBytes,
			Values: nil, Readable: true, Writable: true},
	}
	if got := audibilityPass(elements, DefaultVolume, Any); len(got) != 0 {
		t.Errorf("the pass wrote %v; it should skip read-only, non-mixer and unread elements", got)
	}
}

func TestVolumeOptionRejectsANonPercentage(t *testing.T) {
	if got, err := (Options{}).volume(); err != nil || got != DefaultVolume {
		t.Errorf("the zero value gave (%d, %v), want (%d, nil)", got, err, DefaultVolume)
	}
	if got, err := (Options{Volume: 30}).volume(); err != nil || got != 30 {
		t.Errorf("Volume 30 gave (%d, %v), want (30, nil)", got, err)
	}
	for _, bad := range []int{-1, 101} {
		_, err := (Options{Volume: bad}).volume()
		if err == nil {
			t.Fatalf("Volume %d was accepted", bad)
		}
		if !strings.Contains(err.Error(), "1-100") {
			t.Errorf("error %q does not say what the valid range is", err)
		}
	}
	if _, err := OpenWith(Options{Volume: 200}); err == nil {
		t.Error("OpenWith accepted a Volume of 200")
	}
}

// The dump these produce is the only diagnosis a silent board gets, so it has
// to name the element, decode its value, and say when it cannot be written.
func TestControlAndChangeReadAsDiagnostics(t *testing.T) {
	c := Control{Numid: 13, Iface: IfaceMixer, Name: "Left Headphone Mixer Left DAC Switch",
		Type: ControlBoolean, Max: 1, Values: []int{0}, Readable: true, Writable: true}
	if got := c.String(); !strings.Contains(got, "numid=13") ||
		!strings.Contains(got, `"Left Headphone Mixer Left DAC Switch"`) ||
		!strings.Contains(got, "= off") {
		t.Errorf("boolean control rendered as %q", got)
	}

	e := Control{Numid: 3, Iface: IfaceMixer, Name: "Playback Polarity", Type: ControlEnumerated,
		Items: []string{"Normal", "R Invert"}, Values: []int{1}, Readable: true}
	got := e.String()
	for _, want := range []string{"Normal | R Invert", `1 "R Invert"`, "read-only"} {
		if !strings.Contains(got, want) {
			t.Errorf("enum control rendered as %q, missing %q", got, want)
		}
	}

	ch := Change{Numid: 2, Name: "Headphone Mixer Volume", From: []int{0, 0}, To: []int{8, 8}, Why: "playback level"}
	if got := ch.String(); !strings.Contains(got, "0,0 -> 8,8") || !strings.Contains(got, "playback level") {
		t.Errorf("change rendered as %q", got)
	}
}
