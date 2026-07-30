// Package sound plays PCM audio out of a GoSD board — HDMI on the boards that
// have it, the analog jack on the boards that have one — with no cgo, no
// alsa-lib, and no third-party dependency.
//
// A GoSD image has no userspace at all: no libasound.so.2 to link or dlopen, no
// /usr/share/alsa configuration tree. So this package talks the kernel's ALSA
// PCM ioctl interface directly, which is what alsa-lib's "hw:" plugin does
// underneath. The surface is deliberately small — find a playback device,
// describe the stream, write frames, close:
//
//	dev, err := sound.Open()
//	if err != nil {
//		log.Fatal(err) // the message says how to get a kernel with sound
//	}
//	defer dev.Close()
//	if err := dev.Play(frames); err != nil { ... }
//
// Frames are interleaved signed 16-bit little-endian samples — Format.Channels
// samples per frame, Format.FrameBytes() bytes per frame — which is the one
// format every GoSD board's audio path accepts. There are no decoders here:
// synthesise your samples, or parse a WAV header yourself and hand over its
// PCM payload.
//
// # Sound is not in the stock kernels
//
// Every GoSD board's released kernel is built with `# CONFIG_SOUND is not
// set`, so there is no /dev/snd until you compile a kernel that has it with
// `gosd build-kernel`. Open says so, in those words, when it finds nothing —
// the fix is a custom-kernel recipe, and docs/sound.md holds one per board.
// Two gotchas that doc explains and this package can only report: HDMI audio
// exists only if a display was connected before power-up (the firmware
// enumerates displays once), and the Pi Zero W / Zero 2 W have no analog
// output at all.
//
// See examples/chime for a complete app: synthesis, device choice, and what to
// do when there is no audio device.
package sound

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

// bytesPerSample is the width of one S16_LE sample, the only sample format
// this package speaks.
const bytesPerSample = 2

// ErrNoDevice reports that the board has no audio playback device to open:
// usually a kernel built without sound, sometimes a display that was not
// connected when the board powered up. Errors wrapping it carry the specific
// reason and the fix. Apps that can run without sound should match it with
// errors.Is and carry on.
var ErrNoDevice = errors.New("no audio playback device")

// Format is the shape of a PCM stream: the sample rate in frames per second
// and the channel count. Samples are always signed 16-bit little-endian, so
// one frame is Channels of them.
type Format struct {
	// Rate is the sample rate in Hz — 48000 is what HDMI carries natively.
	Rate int
	// Channels is 2 for stereo, 1 for mono. Most Rockchip codecs and every
	// HDMI sink here want stereo.
	Channels int
}

// FrameBytes is the size of one frame, which is the granularity Play accepts:
// a buffer that is not a whole number of frames is a caller bug, not a partial
// write.
func (f Format) FrameBytes() int { return f.Channels * bytesPerSample }

func (f Format) String() string { return fmt.Sprintf("%d Hz, %d channels", f.Rate, f.Channels) }

// Output names a kind of audio output, for Options.Prefer.
type Output int

const (
	// Any takes whatever the kernel offers first, in card order.
	Any Output = iota
	// HDMI prefers an HDMI (or S/PDIF) sink over an analog one.
	HDMI
	// Analog prefers a headphone/line-out sink over an HDMI one — the right
	// choice on a ROCK 4SE, whose 3.5 mm jack works with no DRM in the
	// kernel while its HDMI audio needs the whole DRM subsystem.
	Analog
)

// Options tunes what Open picks and how it configures it. The zero value is
// what Open uses: any device, 48 kHz stereo, falling back to 44.1 kHz.
type Options struct {
	// Path opens exactly one device, e.g. "/dev/snd/pcmC0D1p", instead of
	// searching. Useful when an app is configured for known hardware.
	Path string
	// Prefer breaks the tie on a board with more than one output.
	Prefer Output
	// Format requests a stream shape. Its zero value asks for 48 kHz
	// stereo and then 44.1 kHz stereo, taking whichever the device accepts.
	Format Format
}

// Device is an open playback PCM. It is an interface because the
// implementation is a Linux syscall client that cannot exist on other hosts —
// and because an app's own tests then have something to fake, which is how
// examples/chime tests its playback logic on macOS.
type Device interface {
	// Play writes interleaved S16_LE frames and returns once they have
	// finished playing. It recovers from underruns itself; any error it
	// does return means the stream is finished, so reopen the device.
	Play(pcm []byte) error
	// Format reports the stream shape Play expects, which is what the
	// device accepted rather than what was asked for.
	Format() Format
	// Name identifies the device for logs — the kernel's name for it where
	// there is one, its /dev/snd path otherwise.
	Name() string
	// Close releases the device.
	Close() error
}

// Open finds the board's best playback device and configures it for 48 kHz
// stereo (or 44.1 kHz if the hardware insists). It reports an error wrapping
// ErrNoDevice, whose message names the fix, when the board has no audio
// device — the usual case, since GoSD's stock kernels have no sound.
func Open() (Device, error) { return OpenWith(Options{}) }

// OpenWith is Open with a choice of device, preferred output and stream
// format.
func OpenWith(opts Options) (Device, error) { return open(opts) }

// formats returns the stream shapes to try, in order: the caller's if it named
// one, otherwise stereo at the two rates every sink here supports.
func (opts Options) formats() []Format {
	if opts.Format.Rate > 0 && opts.Format.Channels > 0 {
		return []Format{opts.Format}
	}
	f := opts.Format
	if f.Channels == 0 {
		f.Channels = 2
	}
	if f.Rate > 0 {
		return []Format{f}
	}
	return []Format{{Rate: 48000, Channels: f.Channels}, {Rate: 44100, Channels: f.Channels}}
}

// pcm is one playback PCM the kernel is exposing.
type pcm struct {
	card, device int
	id, name     string
}

func (p pcm) path() string {
	return fmt.Sprintf("/dev/snd/pcmC%dD%dp", p.card, p.device)
}

func (p pcm) String() string {
	if p.name == "" {
		return p.path()
	}
	return fmt.Sprintf("%s (%s)", p.name, p.path())
}

// isHDMI reports whether this PCM looks like an HDMI or S/PDIF sink. The
// kernel offers no machine-readable flag for it — drivers just name the device
// descriptively ("bcm2835 HDMI 1", "hdmi-sound", "bcm2835 IEC958/HDMI") — so
// the name is all there is to go on.
func (p pcm) isHDMI() bool {
	s := strings.ToUpper(p.id + " " + p.name)
	return strings.Contains(s, "HDMI") || strings.Contains(s, "IEC958")
}

// parsePath reads a /dev/snd/pcmC<card>D<device>p path back into a pcm, so
// Options.Path can skip discovery entirely.
func parsePath(p string) (pcm, error) {
	base := path.Base(p)
	trimmed, ok := strings.CutPrefix(base, "pcmC")
	if !ok || !strings.HasSuffix(trimmed, "p") {
		return pcm{}, fmt.Errorf("%q is not a playback PCM path; it should look like /dev/snd/pcmC0D0p", p)
	}
	card, device, ok := strings.Cut(strings.TrimSuffix(trimmed, "p"), "D")
	if !ok {
		return pcm{}, fmt.Errorf("%q is not a playback PCM path; it should look like /dev/snd/pcmC0D0p", p)
	}
	c, cErr := strconv.Atoi(card)
	d, dErr := strconv.Atoi(device)
	if cErr != nil || dErr != nil {
		return pcm{}, fmt.Errorf("%q has a non-numeric card or device number; it should look like /dev/snd/pcmC0D0p", p)
	}
	return pcm{card: c, device: d}, nil
}

// parseProcPCM reads /proc/asound/pcm, whose lines the kernel formats as
// "%02i-%02i: <id> : <name>" followed by " : playback N" and/or " : capture
// N". Only playback-capable devices are returned.
func parseProcPCM(r io.Reader) []pcm {
	var out []pcm
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Split(strings.TrimSpace(sc.Text()), " : ")
		if len(fields) < 2 {
			continue
		}
		playback := false
		for _, f := range fields[1:] {
			if strings.HasPrefix(f, "playback") {
				playback = true
			}
		}
		if !playback {
			continue
		}
		head := strings.SplitN(fields[0], ": ", 2)
		if len(head) != 2 {
			continue
		}
		nums := strings.SplitN(head[0], "-", 2)
		if len(nums) != 2 {
			continue
		}
		card, err := strconv.Atoi(nums[0])
		if err != nil {
			continue
		}
		device, err := strconv.Atoi(nums[1])
		if err != nil {
			continue
		}
		out = append(out, pcm{card: card, device: device, id: head[1], name: fields[1]})
	}
	return out
}

// parseDevSnd is the fallback when /proc/asound isn't mounted: derive card and
// device numbers from /dev/snd/pcmC<card>D<device>p names, with no idea what
// any of them is called.
func parseDevSnd(paths []string) []pcm {
	var out []pcm
	for _, p := range paths {
		d, err := parsePath(p)
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	return out
}

// rank orders candidates best-first for prefer, then by card and device number
// so the choice is stable across boots.
func rank(devices []pcm, prefer Output) []pcm {
	out := make([]pcm, len(devices))
	copy(out, devices)
	first := func(p pcm) bool {
		switch prefer {
		case HDMI:
			return p.isHDMI()
		case Analog:
			return !p.isHDMI()
		default:
			return false
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if first(out[i]) != first(out[j]) {
			return first(out[i])
		}
		if out[i].card != out[j].card {
			return out[i].card < out[j].card
		}
		return out[i].device < out[j].device
	})
	return out
}

// noDeviceError explains the two very different reasons a board has no
// playback PCM, because the fix for each is completely different: sndPresent
// says whether /dev/snd exists at all, which is the difference between "this
// kernel has no sound" and "sound is there but no card appeared".
func noDeviceError(sndPresent bool) error {
	if !sndPresent {
		return fmt.Errorf("%w: this board has no /dev/snd, so its kernel was built without sound support "+
			"(GoSD's released kernels all are); compile one with `gosd build-kernel` using a recipe from docs/sound.md, "+
			"then build the image with `gosd build --artifacts-dir`", ErrNoDevice)
	}
	return fmt.Errorf("%w: the kernel has sound support but no playback device appeared; "+
		"HDMI audio needs the display connected before power-up, and an analog codec that failed to probe "+
		"shows up in the boot log (see docs/sound.md)", ErrNoDevice)
}
