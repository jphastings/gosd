//go:build linux

package sound

import (
	"testing"
	"unsafe"
)

// The kernel decides what these structs mean, and the ioctl request number
// encodes their size, so a layout that is wrong by one padding byte is not a
// subtle bug: it is a different ioctl. These are the sizes and offsets of
// snd_pcm_hw_params, snd_pcm_sw_params and snd_xferi as a C compiler lays them
// out for a 64-bit and a 32-bit Linux ABI, which is exactly the pair of
// architectures GoSD boards use (arm64, and armv6 for the Pi Zero W).
func TestStructLayoutMatchesKernelABI(t *testing.T) {
	var (
		hw pcmHWParams
		sw pcmSWParams
		xi pcmXferI
	)

	if got, want := unsafe.Sizeof(pcmMask{}), uintptr(32); got != want {
		t.Errorf("sizeof(snd_mask) = %d, want %d", got, want)
	}
	if got, want := unsafe.Sizeof(pcmInterval{}), uintptr(12); got != want {
		t.Errorf("sizeof(snd_interval) = %d, want %d", got, want)
	}

	type layout struct {
		hwSize, hwFifoOff   uintptr
		swSize, swAvailOff  uintptr
		xferSize, xferBufOf uintptr
	}
	var want layout
	switch word := unsafe.Sizeof(uintptr(0)); word {
	case 8:
		want = layout{hwSize: 608, hwFifoOff: 536, swSize: 136, swAvailOff: 16, xferSize: 24, xferBufOf: 8}
	case 4:
		want = layout{hwSize: 604, hwFifoOff: 536, swSize: 104, swAvailOff: 12, xferSize: 12, xferBufOf: 4}
	default:
		t.Fatalf("unexpected word size %d: the ALSA ABI here assumes 32- or 64-bit", word)
	}

	if got := unsafe.Sizeof(hw); got != want.hwSize {
		t.Errorf("sizeof(snd_pcm_hw_params) = %d, want %d", got, want.hwSize)
	}
	if got := unsafe.Offsetof(hw.fifoSize); got != want.hwFifoOff {
		t.Errorf("offsetof(snd_pcm_hw_params.fifo_size) = %d, want %d", got, want.hwFifoOff)
	}
	if got := unsafe.Sizeof(sw); got != want.swSize {
		t.Errorf("sizeof(snd_pcm_sw_params) = %d, want %d", got, want.swSize)
	}
	if got := unsafe.Offsetof(sw.availMin); got != want.swAvailOff {
		t.Errorf("offsetof(snd_pcm_sw_params.avail_min) = %d, want %d", got, want.swAvailOff)
	}
	if got := unsafe.Sizeof(xi); got != want.xferSize {
		t.Errorf("sizeof(snd_xferi) = %d, want %d", got, want.xferSize)
	}
	if got := unsafe.Offsetof(xi.buf); got != want.xferBufOf {
		t.Errorf("offsetof(snd_xferi.buf) = %d, want %d", got, want.xferBufOf)
	}
}

// The PREPARE and DRAIN numbers carry no struct, so they are stable constants
// that can be checked against the header outright; HW_PARAMS is size-dependent
// and so is checked against the size this build actually uses.
func TestIoctlRequestEncoding(t *testing.T) {
	if got, want := reqPrepare, uintptr(0x00004140); got != want {
		t.Errorf("SNDRV_PCM_IOCTL_PREPARE = %#x, want %#x", got, want)
	}
	if got, want := reqDrain, uintptr(0x00004144); got != want {
		t.Errorf("SNDRV_PCM_IOCTL_DRAIN = %#x, want %#x", got, want)
	}
	wantHW := uintptr(3)<<30 | unsafe.Sizeof(pcmHWParams{})<<16 | 'A'<<8 | 0x11
	if reqHWParams != wantHW {
		t.Errorf("SNDRV_PCM_IOCTL_HW_PARAMS = %#x, want %#x", reqHWParams, wantHW)
	}
	if got, want := reqPVersion, uintptr(2)<<30|4<<16|'A'<<8; got != want {
		t.Errorf("SNDRV_PCM_IOCTL_PVERSION = %#x, want %#x", got, want)
	}
}

func TestAnyHWParamsIsFullyOpenUntilConstrained(t *testing.T) {
	p := anyHWParams()
	if p.masks[maskFormat].bits[0] != ^uint32(0) {
		t.Error("format mask should start with every format allowed")
	}
	if got := p.intervals[intervalRate]; got.min != 0 || got.max != ^uint32(0) {
		t.Errorf("rate interval starts as %v, want the full range", got)
	}

	p.pin(maskFormat, formatS16LE)
	if got, want := p.masks[maskFormat].bits[0], uint32(1<<formatS16LE); got != want {
		t.Errorf("pinned format mask = %#x, want %#x", got, want)
	}
	for i := 1; i < len(p.masks[maskFormat].bits); i++ {
		if p.masks[maskFormat].bits[i] != 0 {
			t.Errorf("pinning left word %d of the format mask set", i)
		}
	}

	p.bound(intervalRate, 48000, 48000)
	if got := p.intervals[intervalRate]; got.min != 48000 || got.max != 48000 || got.flags&intervalInteger == 0 {
		t.Errorf("bounded rate interval = %+v, want 48000..48000 with the integer flag", got)
	}
	if got := p.chosen(intervalRate); got != 48000 {
		t.Errorf("chosen(rate) = %d, want 48000", got)
	}
}

// A mask value above 31 has to land in the right word of the bitmap — the
// PCM format enumeration runs well past 31 (S24_3LE and friends), so getting
// this wrong silently asks for the wrong format.
func TestPinHandlesValuesBeyondTheFirstWord(t *testing.T) {
	p := anyHWParams()
	p.pin(maskFormat, 34)
	if p.masks[maskFormat].bits[0] != 0 {
		t.Error("word 0 should be clear")
	}
	if got, want := p.masks[maskFormat].bits[1], uint32(1<<2); got != want {
		t.Errorf("word 1 = %#x, want %#x", got, want)
	}
}
