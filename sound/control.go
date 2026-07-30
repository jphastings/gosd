package sound

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// This file is the portable half of ALSA control (mixer) support: the types an
// app sees, and the rules Open follows to make a freshly powered-up card
// audible. The ioctl transport is in control_linux.go, so everything here is
// exercised by tests on a developer's Mac.

// DefaultVolume is the playback level Open sets when Options.Volume is zero,
// as a percentage of each playback control's range.
//
// Three quarters, not full scale: the top of an analog output's range is where
// its own distortion lives — the pi-3b's jack is PWM straight from the SoC,
// and a codec's headphone amp clips before its DAC does — while anything
// timid enough to need a quiet room defeats the point of a device that has to
// be heard without anyone present to turn it up. It is also only ever a floor;
// see Options.Volume.
const DefaultVolume = 75

// Iface is the ALSA interface a control element belongs to. Open's audibility
// pass only ever touches IfaceMixer elements.
type Iface int

// The SNDRV_CTL_ELEM_IFACE_* values.
const (
	IfaceCard Iface = iota
	IfaceHWDep
	IfaceMixer
	IfacePCM
	IfaceRawMIDI
	IfaceTimer
	IfaceSequencer
)

func (i Iface) String() string {
	switch i {
	case IfaceCard:
		return "CARD"
	case IfaceHWDep:
		return "HWDEP"
	case IfaceMixer:
		return "MIXER"
	case IfacePCM:
		return "PCM"
	case IfaceRawMIDI:
		return "RAWMIDI"
	case IfaceTimer:
		return "TIMER"
	case IfaceSequencer:
		return "SEQUENCER"
	default:
		return fmt.Sprintf("IFACE(%d)", int(i))
	}
}

// ControlType is the value type of a control element.
type ControlType int

// The SNDRV_CTL_ELEM_TYPE_* values.
const (
	ControlNone ControlType = iota
	ControlBoolean
	ControlInteger
	ControlEnumerated
	ControlBytes
	ControlIEC958
	ControlInteger64
)

func (t ControlType) String() string {
	switch t {
	case ControlBoolean:
		return "boolean"
	case ControlInteger:
		return "integer"
	case ControlEnumerated:
		return "enum"
	case ControlBytes:
		return "bytes"
	case ControlIEC958:
		return "iec958"
	case ControlInteger64:
		return "integer64"
	default:
		return "none"
	}
}

// Control is one ALSA control element — a volume, a mute switch, a routing
// mux — as the kernel describes it, with the value it held when it was read.
type Control struct {
	// Numid is the kernel's numeric identifier for this element on its card.
	// It is what `amixer cget numid=N` takes, and it is stable for the life
	// of the card.
	Numid int
	// Iface is the element's interface. Mixer controls are IfaceMixer;
	// per-PCM things like IEC958 status bits are IfacePCM.
	Iface Iface
	// Name is the element's name, e.g. "PCM Playback Volume".
	Name string
	// Index distinguishes elements that share a name.
	Index int
	// Type says how to read Values.
	Type ControlType
	// Values is one value per channel: 0 or 1 for a boolean, the raw value
	// for an integer, an index into Items for an enumerated element. Byte
	// and IEC958 elements are not read, so their Values is empty.
	Values []int
	// Min, Max and Step bound an integer element's Values. A boolean's
	// range is always 0..1.
	Min, Max, Step int
	// Items names an enumerated element's choices.
	Items []string
	// Readable and Writable are the element's access bits.
	Readable, Writable bool
	// Inactive reports that the driver has marked the element inactive —
	// usually DAPM saying the path it belongs to is powered down. Worth
	// knowing when a control looks right but nothing plays.
	Inactive bool
}

func (c Control) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "numid=%d %s %q %s", c.Numid, c.Iface, c.Name, c.Type)
	if c.Index != 0 {
		fmt.Fprintf(&b, " index=%d", c.Index)
	}
	switch c.Type {
	case ControlInteger, ControlInteger64:
		fmt.Fprintf(&b, " %d..%d", c.Min, c.Max)
		if c.Step > 1 {
			fmt.Fprintf(&b, " step %d", c.Step)
		}
	case ControlEnumerated:
		fmt.Fprintf(&b, " [%s]", strings.Join(c.Items, " | "))
	}
	fmt.Fprintf(&b, " = %s", c.valueString())
	if !c.Writable {
		b.WriteString(" read-only")
	}
	if c.Inactive {
		b.WriteString(" INACTIVE")
	}
	return b.String()
}

func (c Control) valueString() string {
	if len(c.Values) == 0 {
		return "(not read)"
	}
	parts := make([]string, len(c.Values))
	for i, v := range c.Values {
		switch {
		case c.Type == ControlBoolean && v != 0:
			parts[i] = "on"
		case c.Type == ControlBoolean:
			parts[i] = "off"
		case c.Type == ControlEnumerated && v >= 0 && v < len(c.Items):
			parts[i] = fmt.Sprintf("%d %q", v, c.Items[v])
		default:
			parts[i] = strconv.Itoa(v)
		}
	}
	return strings.Join(parts, ", ")
}

// Change is one control element Open's audibility pass wrote, and why.
type Change struct {
	// Numid and Name identify the element, as on Control.
	Numid int
	Name  string
	// From and To are the element's values before and after the write.
	From, To []int
	// Why names the rule that fired, for a log line that explains itself.
	Why string
}

func (c Change) String() string {
	return fmt.Sprintf("numid=%d %q: %s -> %s (%s)", c.Numid, c.Name, ints(c.From), ints(c.To), c.Why)
}

func ints(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

// Mixer is a snapshot of a card's control elements, plus a record of what Open
// changed to make the card audible. It is what a board bring-up log should
// print when a tone plays but nothing is heard.
type Mixer struct {
	// Card is the ALSA card number the elements belong to.
	Card int
	// Elements is every control element on the card, in kernel order.
	Elements []Control
	// Changed is what Open's audibility pass wrote, in the order it wrote
	// it. Empty when Options.SkipMixer was set, or when nothing needed
	// changing.
	Changed []Change
}

// Word lists for the naming heuristic.
//
// ALSA control names follow a loose convention — "<SOURCE> [DIRECTION]
// <FUNCTION>", as in "PCM Playback Volume" — but ASoC codec drivers add plenty
// of names the convention never anticipated ("DAC Notch Filter Switch",
// "DAC Mono Mix Switch"), and flipping one of those is worse than leaving a
// control alone. So matching is on whole words, the lists are short, and a
// name has to look like an output path *and* like a level or a mute before
// anything is written to it.
var (
	// captureWords disqualify an element outright: they name the input
	// path, which this package never touches.
	captureWords = []string{
		"capture", "mic", "adc", "alc", "input", "in", "aux",
		"sidetone", "loopback", "monitor", "bypass", "boost",
	}
	// digitalWords name the DAC or PCM stream — the source that has to
	// reach an output stage for anything to be heard.
	digitalWords = []string{"dac", "pcm", "i2s", "digital"}
	// outputWords name an analog output stage.
	outputWords = []string{
		"headphone", "headphones", "hp", "hpout", "speaker", "speakers",
		"spk", "lineout", "master", "output", "out",
	}
	// hdmiWords and analogWords match the choices of a routing control
	// against Options.Prefer.
	hdmiWords   = []string{"hdmi", "iec958", "spdif", "displayport", "dp"}
	analogWords = []string{"analog", "analogue", "headphone", "headphones", "hp", "jack", "speaker", "speakers", "spk", "line", "lineout"}
)

// words splits a control or item name into lowercase words. Whole-word
// matching is what stops "lin1-rin1" reading as an output ("in") by accident
// and "LDATA TO LDAC" reading as a DAC source.
func words(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func hasWord(in []string, any ...string) bool {
	for _, w := range in {
		for _, want := range any {
			if w == want {
				return true
			}
		}
	}
	return false
}

// function is the last word of a control's name, which by convention is what
// the element does: "Volume", "Switch", "Route".
func function(in []string) string {
	if len(in) == 0 {
		return ""
	}
	return in[len(in)-1]
}

// audibilityPass decides what to write to a card's controls so that a stream
// written to its PCM is actually heard. ALSA codecs power up muted, at zero,
// with their output mixers disconnected — alsamixer and alsactl are what
// normally fix that, and a GoSD image has neither.
func audibilityPass(elements []Control, volume int, prefer Output) []Change {
	routed, routeCh, hasRoute := routeChange(elements, prefer)
	var changes []Change
	for i, e := range elements {
		if hasRoute && i == routed {
			continue
		}
		if ch, ok := elementChange(e, volume); ok {
			changes = append(changes, ch)
		}
	}
	if hasRoute {
		changes = append(changes, routeCh)
	}
	return changes
}

func elementChange(c Control, volume int) (Change, bool) {
	if !c.Writable || c.Iface != IfaceMixer || len(c.Values) == 0 {
		return Change{}, false
	}
	name := words(c.Name)
	if hasWord(name, captureWords...) {
		return Change{}, false
	}
	switch c.Type {
	case ControlBoolean:
		return switchChange(c, name)
	case ControlInteger:
		return volumeChange(c, name, volume)
	case ControlEnumerated:
		return sourceChange(c, name)
	default:
		return Change{}, false
	}
}

// switchChange turns on a mute switch or an output mixer's DAC input.
//
// "<x> Playback Switch" is the convention's mute switch. The other shape that
// matters is a DAPM output-mixer input — the kernel names those "<mixer
// widget> <input> Switch", e.g. the ES8316's "Left Headphone Mixer Left DAC
// Switch", which powers up muted and is exactly why a ROCK 4SE plays silence.
// Requiring both a digital source and an output stage in the name is what
// keeps this rule off "DAC Notch Filter Switch" and off the same mixer's
// "LLIN Switch", which would fold the mic input into the headphones.
func switchChange(c Control, name []string) (Change, bool) {
	if function(name) != "switch" {
		return Change{}, false
	}
	playbackPath := hasWord(name, digitalWords...) && hasWord(name, outputWords...)
	if !hasWord(name, "playback") && !playbackPath {
		return Change{}, false
	}
	return raise(c, func(int) int { return 1 }, "unmute the playback path")
}

// volumeChange raises a playback level to at least volume% of its range.
//
// It never turns anything down. A percentage of a control's raw range is a
// blunt instrument — the ES8316's "DAC Playback Volume" spans -96 dB in 192
// steps and powers up at 0 dB, and the Pi's "PCM Playback Volume" spans
// -102.39 dB and powers up at 0 dB — so setting either to a fraction of range
// would *attenuate* a driver default that was already right. Since nobody is
// at the bench to turn a GoSD board back up, the pass only ever raises.
func volumeChange(c Control, name []string, volume int) (Change, bool) {
	if function(name) != "volume" {
		return Change{}, false
	}
	if !hasWord(name, "playback") && !hasWord(name, digitalWords...) && !hasWord(name, outputWords...) {
		return Change{}, false
	}
	if c.Max <= c.Min {
		return Change{}, false
	}
	target := c.Min + int(int64(c.Max-c.Min)*int64(volume)/100)
	return raise(c, func(was int) int {
		if target > was {
			return target
		}
		return was
	}, fmt.Sprintf("playback level, at least %d%% of range", volume))
}

// sourceChange points an output mux at the DAC, where the mux names one.
// Codecs whose output stage selects between the DAC and an analog input come
// up on whichever the reset default was; an enum with no DAC-ish choice
// (polarity, an analog-input mux) is left alone.
func sourceChange(c Control, name []string) (Change, bool) {
	if !hasWord(name, "playback") && !hasWord(name, digitalWords...) && !hasWord(name, outputWords...) {
		return Change{}, false
	}
	want := -1
	for i, item := range c.Items {
		if hasWord(words(item), digitalWords...) {
			want = i
			break
		}
	}
	if want < 0 {
		return Change{}, false
	}
	return raise(c, func(was int) int {
		if was >= 0 && was < len(c.Items) && hasWord(words(c.Items[was]), digitalWords...) {
			return was
		}
		return want
	}, fmt.Sprintf("route the DAC to the output (%q)", c.Items[want]))
}

// routeChange honours Options.Prefer on a card that routes its outputs with a
// control rather than by exposing them as separate PCMs. Only enumerated route
// controls are touched: an integer one's values mean whatever its driver says
// they mean, and guessing would be worse than leaving it where the driver put
// it.
func routeChange(elements []Control, prefer Output) (int, Change, bool) {
	if prefer == Any {
		return 0, Change{}, false
	}
	for i, c := range elements {
		if !c.Writable || c.Iface != IfaceMixer || c.Type != ControlEnumerated {
			continue
		}
		name := words(c.Name)
		if function(name) != "route" || hasWord(name, captureWords...) {
			continue
		}
		want := -1
		for j, item := range c.Items {
			if prefer == HDMI && hasWord(words(item), hdmiWords...) {
				want = j
				break
			}
			if prefer == Analog && hasWord(words(item), analogWords...) {
				want = j
				break
			}
		}
		if want < 0 {
			continue
		}
		ch, ok := raise(c, func(int) int { return want },
			fmt.Sprintf("Options.Prefer is %s, so route to %q", prefer, c.Items[want]))
		if ok {
			return i, ch, true
		}
	}
	return 0, Change{}, false
}

// raise builds the Change that applies want to every one of an element's
// values, and reports false when that would leave the element unchanged.
func raise(c Control, want func(was int) int, why string) (Change, bool) {
	to := make([]int, len(c.Values))
	changed := false
	for i, was := range c.Values {
		to[i] = want(was)
		if to[i] != was {
			changed = true
		}
	}
	if !changed {
		return Change{}, false
	}
	from := make([]int, len(c.Values))
	copy(from, c.Values)
	return Change{Numid: c.Numid, Name: c.Name, From: from, To: to, Why: why}, true
}
