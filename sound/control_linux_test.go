//go:build linux

package sound

import (
	"testing"
	"unsafe"
)

// The ioctl request numbers for the control interface encode the size of the
// struct they carry (see ctlReq), so these structs being the wrong size is
// not a layout nicety: ELEM_INFO and ELEM_READ/WRITE become different ioctls,
// and every Open() issues them as part of its audibility pass. control_linux.go
// already refuses to compile unless each struct matches a size *formula*, but a
// formula written alongside the struct it checks can carry the same mistake
// twice, so these numbers are laid out from the C declarations in
// include/uapi/sound/asound.h instead:
//
//	snd_ctl_elem_id     4+4+4+4+44+4                              = 64
//	snd_ctl_elem_list   16 + a pointer + reserved[50], padded     = 80 / 72
//	snd_ctl_elem_info   64 + 16 + a 128-byte union + reserved[64] = 272
//	snd_ctl_elem_value  64 + 4 (+4 pad) + long value[128] + 128   = 1224 / 712
//
// where a pair is (64-bit, 32-bit) — arm64 and the Pi Zero W's armv6, the two
// ABIs GoSD builds for. The union in snd_ctl_elem_info is 8-aligned and 128
// bytes wide on both (its widest arm is reserved[128]); the one in
// snd_ctl_elem_value is the "long value[128]" that changes width with the
// word, which is exactly why the two totals differ.
func TestControlStructLayoutMatchesKernelABI(t *testing.T) {
	word := unsafe.Sizeof(uintptr(0))

	var wantList, wantValue uintptr
	switch word {
	case 8:
		wantList, wantValue = 80, 1224
	case 4:
		wantList, wantValue = 72, 712
	default:
		t.Fatalf("unexpected word size %d: the ALSA ABI here assumes 32- or 64-bit", word)
	}

	for _, tc := range []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"snd_ctl_elem_id", unsafe.Sizeof(ctlElemID{}), 64},
		{"snd_ctl_elem_list", unsafe.Sizeof(ctlElemList{}), wantList},
		{"snd_ctl_elem_info", unsafe.Sizeof(ctlElemInfo{}), 272},
		{"snd_ctl_elem_value", unsafe.Sizeof(ctlElemValue{}), wantValue},
	} {
		if tc.got != tc.want {
			t.Errorf("sizeof(%s) = %d, want %d", tc.name, tc.got, tc.want)
		}
	}

	// Offsets the accessors index by hand, and so cannot check themselves.
	var (
		info  ctlElemInfo
		value ctlElemValue
		id    ctlElemID
	)
	if got, want := unsafe.Offsetof(id.name), uintptr(16); got != want {
		t.Errorf("offsetof(snd_ctl_elem_id.name) = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(info.value), uintptr(80); got != want {
		t.Errorf("offsetof(snd_ctl_elem_info.value) = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(value.value), uintptr(72); got != want {
		t.Errorf("offsetof(snd_ctl_elem_value.value) = %d, want %d", got, want)
	}
	// The enumerated arm is read out of that union in place, so it has to fit
	// inside it: items + item + name[64].
	if got, want := unsafe.Sizeof(ctlEnumInfo{}), uintptr(72); got != want {
		t.Errorf("sizeof(snd_ctl_elem_info.value.enumerated) = %d, want %d", got, want)
	}
}
