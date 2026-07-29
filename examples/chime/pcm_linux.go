//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// This is the ALSA PCM kernel ABI, transcribed from
// include/uapi/sound/asound.h, and the blocking playback path over it.
//
// GoSD images have no userspace: no libasound, no /usr/share/alsa, nothing to
// dlopen. Playback therefore talks to /dev/snd/pcmC<card>D<device>p directly,
// which is what alsa-lib's "hw:" plugin does underneath. The minimal path is
// HW_PARAMS -> SW_PARAMS -> PREPARE -> WRITEI_FRAMES in a loop: playback
// auto-starts once start_threshold frames are queued, so START is never
// issued, and an underrun (EPIPE) is recovered by re-issuing PREPARE.

// pcmVersion is SNDRV_PCM_VERSION, the PCM protocol version this code was
// written against (2.0.18 — unchanged for over a decade). Only the major
// number is enforced: within a major version the kernel only appends to the
// tails of these structs.
const pcmVersion = uint32(2<<16 | 0<<8 | 18)

func pcmVersionMajor(v uint32) uint32 { return v >> 16 }

// snd_mask / snd_interval. The C bitfields openmin:1, openmax:1, integer:1,
// empty:1 have no Go equivalent, so they are one uint32 whose bit order is the
// little-endian ABI's: LSB first, in declaration order.
type pcmMask struct {
	bits [8]uint32
}

type pcmInterval struct {
	min, max uint32
	flags    uint32
}

const intervalInteger uint32 = 1 << 2

// pcmHWParams is struct snd_pcm_hw_params. Every snd_pcm_uframes_t
// (C unsigned long) field is uintptr, which is the native word width on both
// GOARCH=arm and GOARCH=arm64 — see ioctlReq for why that matters more than it
// looks.
type pcmHWParams struct {
	flags     uint32
	masks     [3]pcmMask
	mres      [5]pcmMask
	intervals [12]pcmInterval
	ires      [9]pcmInterval
	rmask     uint32
	cmask     uint32
	info      uint32
	msbits    uint32
	rateNum   uint32
	rateDen   uint32
	fifoSize  uintptr
	sync      [16]byte
	reserved  [48]byte
}

// pcmSWParams is struct snd_pcm_sw_params.
type pcmSWParams struct {
	tstampMode       int32
	periodStep       uint32
	sleepMin         uint32
	availMin         uintptr
	xferAlign        uintptr
	startThreshold   uintptr
	stopThreshold    uintptr
	silenceThreshold uintptr
	silenceSize      uintptr
	boundary         uintptr
	proto            uint32
	tstampType       uint32
	reserved         [56]byte
}

// pcmXferI is struct snd_xferi, the argument to WRITEI_FRAMES. buf holds a
// pointer to Go memory as a uintptr, which the garbage collector cannot see:
// Play keeps the referenced buffer alive across the ioctl with
// runtime.KeepAlive, and relies on Go's collector not relocating heap objects.
type pcmXferI struct {
	result int
	buf    uintptr
	frames uintptr
}

// The ioctl request number embeds sizeof(struct) (see ioctlReq), so a layout
// that is one padding byte out on one architecture is not a subtle bug there:
// it is a different ioctl. These three lines fail to *compile* unless each
// struct is exactly the size a C compiler would give it, which means the armv6
// layout is checked by the same cross-compiles CI already runs and not only by
// a test on whatever machine runs the suite.
const (
	word = unsafe.Sizeof(uintptr(0))
	// snd_pcm_hw_params: 536 bytes of flags, masks and intervals, then one
	// snd_pcm_uframes_t, then sync[16] + reserved[48].
	wantHWParamsSize = 536 + word + 64
	// snd_pcm_sw_params: three ints padded up to the word, seven
	// snd_pcm_uframes_t, two uints and reserved[56].
	wantSWParamsSize = ((12+word-1)/word)*word + 7*word + 64
	// snd_xferi: a long, a pointer and an unsigned long.
	wantXferISize = 3 * word
)

var (
	_ = [1]struct{}{}[unsafe.Sizeof(pcmHWParams{})-wantHWParamsSize]
	_ = [1]struct{}{}[unsafe.Sizeof(pcmSWParams{})-wantSWParamsSize]
	_ = [1]struct{}{}[unsafe.Sizeof(pcmXferI{})-wantXferISize]
)

// Mask and interval indices. SNDRV_PCM_HW_PARAM_ACCESS..SUBFORMAT index
// masks[]; the interval parameters start at SAMPLE_BITS = 8, so
// intervals[param-8].
const (
	maskAccess = 0
	maskFormat = 1

	intervalChannels   = 10 - 8
	intervalRate       = 11 - 8
	intervalPeriodTime = 12 - 8
	intervalPeriodSize = 13 - 8
	intervalBufferTime = 16 - 8
	intervalBufferSize = 17 - 8
)

const (
	accessRWInterleaved = 3 // SNDRV_PCM_ACCESS_RW_INTERLEAVED
	formatS16LE         = 2 // SNDRV_PCM_FORMAT_S16_LE
)

// ioctl direction bits and the PCM ioctl type ('A').
const (
	iocWrite uintptr = 1
	iocRead  uintptr = 2
	iocMagic uintptr = 'A'
)

// ioctlReq encodes Linux's _IOC(dir, type, nr, size). The size field is the
// reason none of these numbers can be hardcoded: snd_pcm_hw_params is 608
// bytes on arm64 and 604 on armv6 (snd_pcm_uframes_t is an unsigned long), so
// a constant lifted from a 64-bit header is simply the wrong ioctl on
// GOARCH=arm. Deriving size from unsafe.Sizeof keeps the number consistent
// with whatever struct is actually passed.
func ioctlReq(dir, size, nr uintptr) uintptr {
	return dir<<30 | size<<16 | iocMagic<<8 | nr
}

var (
	reqPVersion  = ioctlReq(iocRead, unsafe.Sizeof(int32(0)), 0x00)
	reqHWParams  = ioctlReq(iocRead|iocWrite, unsafe.Sizeof(pcmHWParams{}), 0x11)
	reqSWParams  = ioctlReq(iocRead|iocWrite, unsafe.Sizeof(pcmSWParams{}), 0x13)
	reqPrepare   = ioctlReq(0, 0, 0x40)
	reqDrain     = ioctlReq(0, 0, 0x44)
	reqWriteIFrm = ioctlReq(iocWrite, unsafe.Sizeof(pcmXferI{}), 0x50)
)

// anyHWParams is snd_pcm_hw_params_any: every mask bit set and every interval
// wide open, so the kernel's refine step intersects them with what the
// hardware can actually do. HW_PARAMS runs that refine internally and then
// picks concrete values, so the separate HW_REFINE round-trip alsa-lib makes
// is unnecessary here.
func anyHWParams() *pcmHWParams {
	p := &pcmHWParams{rmask: ^uint32(0), info: ^uint32(0)}
	for i := range p.masks {
		for j := range p.masks[i].bits {
			p.masks[i].bits[j] = ^uint32(0)
		}
	}
	for i := range p.intervals {
		p.intervals[i] = pcmInterval{min: 0, max: ^uint32(0)}
	}
	return p
}

// pin narrows a mask to exactly one value.
func (p *pcmHWParams) pin(mask, value int) {
	for i := range p.masks[mask].bits {
		p.masks[mask].bits[i] = 0
	}
	p.masks[mask].bits[value>>5] = 1 << uint(value&31)
}

// bound constrains an interval to [minVal, maxVal] whole values.
func (p *pcmHWParams) bound(interval int, minVal, maxVal uint32) {
	p.intervals[interval] = pcmInterval{min: minVal, max: maxVal, flags: intervalInteger}
}

// chosen reads back the single value the kernel settled on for an interval.
func (p *pcmHWParams) chosen(interval int) uint32 { return p.intervals[interval].min }

// rates are tried in order until the device accepts one. 48 kHz is what HDMI
// carries natively; 44.1 kHz is the usual fallback.
var rates = []int{48000, 44100}

// device is an open playback PCM.
type device struct {
	file   *os.File
	dev    pcmDevice
	format format
	period int // frames per period, as chosen by the kernel
}

// openSink finds a usable playback PCM and configures it. want, if non-empty,
// forces one device path instead of searching.
func openSink(want string) (sink, error) {
	candidates, err := listDevices(want)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, errors.New("no playback PCM devices found under /dev/snd")
	}
	var lastErr error
	for _, c := range candidates {
		d, err := openDevice(c)
		if err != nil {
			lastErr = err
			continue
		}
		return d, nil
	}
	return nil, lastErr
}

// listDevices prefers /proc/asound/pcm, which names each PCM (the only way to
// tell an HDMI sink from an analog one), and falls back to the /dev/snd node
// names if /proc isn't mounted.
func listDevices(want string) ([]pcmDevice, error) {
	if want != "" {
		var d pcmDevice
		if _, err := fmt.Sscanf(filepath.Base(want), "pcmC%dD%dp", &d.card, &d.device); err != nil {
			return nil, fmt.Errorf("CHIME_DEVICE=%q is not a /dev/snd/pcmC<card>D<device>p path", want)
		}
		return []pcmDevice{d}, nil
	}
	if f, err := os.Open("/proc/asound/pcm"); err == nil {
		defer func() { _ = f.Close() }()
		if devices := parseProcPCM(f); len(devices) > 0 {
			return rank(devices), nil
		}
	}
	paths, err := filepath.Glob("/dev/snd/pcmC*D*p")
	if err != nil {
		return nil, fmt.Errorf("looking for PCM devices in /dev/snd: %w", err)
	}
	return rank(parseDevSnd(paths)), nil
}

func openDevice(dev pcmDevice) (*device, error) {
	file, err := os.OpenFile(dev.path(), unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", dev.path(), err)
	}
	d := &device{file: file, dev: dev}

	var version uint32
	if err := d.ioctl(reqPVersion, unsafe.Pointer(&version)); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("reading the PCM protocol version of %s: %w", dev.path(), err)
	}
	if pcmVersionMajor(version) != pcmVersionMajor(pcmVersion) {
		_ = file.Close()
		return nil, fmt.Errorf("%s speaks PCM protocol %d.%d.%d, this build understands major %d only",
			dev.path(), version>>16, (version>>8)&0xff, version&0xff, pcmVersionMajor(pcmVersion))
	}

	var lastErr error
	for _, rate := range rates {
		if err := d.configure(format{rate: rate, channels: 2}, version); err != nil {
			lastErr = err
			continue
		}
		return d, nil
	}
	_ = file.Close()
	return nil, fmt.Errorf("no supported format on %s: %w", dev.path(), lastErr)
}

// configure commits hardware and software parameters and prepares the stream.
// Rate, channel count, format and access are pinned; period and buffer sizes
// are left as ranges so the kernel picks whatever the hardware likes within
// sane latency bounds.
func (d *device) configure(f format, version uint32) error {
	hw := anyHWParams()
	hw.pin(maskAccess, accessRWInterleaved)
	hw.pin(maskFormat, formatS16LE)
	hw.bound(intervalChannels, uint32(f.channels), uint32(f.channels))
	hw.bound(intervalRate, uint32(f.rate), uint32(f.rate))
	hw.bound(intervalPeriodTime, 10_000, 100_000)
	hw.bound(intervalBufferTime, 40_000, 400_000)
	if err := d.ioctl(reqHWParams, unsafe.Pointer(hw)); err != nil {
		return fmt.Errorf("setting %d Hz %d-channel S16_LE: %w", f.rate, f.channels, err)
	}
	period := hw.chosen(intervalPeriodSize)
	buffer := hw.chosen(intervalBufferSize)
	if period == 0 || buffer == 0 {
		return fmt.Errorf("kernel chose a zero-sized period (%d) or buffer (%d)", period, buffer)
	}

	sw := &pcmSWParams{
		periodStep: 1,
		availMin:   uintptr(period),
		xferAlign:  1,
		// Start as soon as one period is queued, and treat the buffer
		// running dry as an underrun so Play recovers deliberately.
		startThreshold: uintptr(period),
		stopThreshold:  uintptr(buffer),
		proto:          version,
	}
	if err := d.ioctl(reqSWParams, unsafe.Pointer(sw)); err != nil {
		return fmt.Errorf("setting software parameters: %w", err)
	}
	if err := d.prepare(); err != nil {
		return err
	}
	d.format = f
	d.period = int(period)
	return nil
}

func (d *device) prepare() error {
	if err := d.ioctl(reqPrepare, nil); err != nil {
		return fmt.Errorf("preparing the stream: %w", err)
	}
	return nil
}

// Play writes interleaved S16_LE frames and waits for them to finish playing.
func (d *device) Play(pcm []byte) error {
	frameBytes := d.format.frameBytes()
	if len(pcm)%frameBytes != 0 {
		return fmt.Errorf("%d bytes is not a whole number of %d-byte frames", len(pcm), frameBytes)
	}
	for off := 0; off < len(pcm); {
		x := pcmXferI{
			buf:    uintptr(unsafe.Pointer(&pcm[off])),
			frames: uintptr((len(pcm) - off) / frameBytes),
		}
		err := d.ioctl(reqWriteIFrm, unsafe.Pointer(&x))
		runtime.KeepAlive(pcm)
		switch {
		case errors.Is(err, unix.EPIPE):
			// Underrun: the kernel stopped the stream. Re-prepare and
			// carry on from where we got to.
			if err := d.prepare(); err != nil {
				return err
			}
			continue
		case err != nil:
			return fmt.Errorf("writing %d frames: %w", x.frames, err)
		case x.result <= 0:
			return fmt.Errorf("wrote %d frames", x.result)
		}
		off += x.result * frameBytes
	}
	if err := d.ioctl(reqDrain, nil); err != nil {
		return fmt.Errorf("draining the stream: %w", err)
	}
	// DRAIN leaves the stream in SETUP, so the next Play needs a fresh
	// PREPARE.
	return d.prepare()
}

func (d *device) Format() format { return d.format }

func (d *device) Name() string { return d.dev.String() }

func (d *device) Close() error { return d.file.Close() }

// ioctl issues one ioctl against the PCM. The unsafe.Pointer to uintptr
// conversion is written inside the call expression, which is the only form the
// compiler treats as keeping the referent alive for the duration of the call.
func (d *device) ioctl(req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, d.file.Fd(), req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}
