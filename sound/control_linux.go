//go:build linux

package sound

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// This is the ALSA control (mixer) kernel ABI, transcribed from
// include/uapi/sound/asound.h, and the read/modify path over it.
//
// A control device is /dev/snd/controlC<card>. Enumerating it is ELEM_LIST
// (once for the count, once for the ids), then ELEM_INFO and ELEM_READ per
// element; changing one is ELEM_WRITE. That is the whole of what alsamixer
// does, minus the curses.

// ctlVersion is SNDRV_CTL_VERSION, the control protocol version this code was
// written against (2.0.9). As with the PCM path only the major number is
// enforced: within a major version the kernel only appends.
const ctlVersion = uint32(2<<16 | 0<<8 | 9)

// ctlNameLen is SNDRV_CTL_ELEM_ID_NAME_MAXLEN.
const ctlNameLen = 44

// ctlElemID is struct snd_ctl_elem_id.
type ctlElemID struct {
	numid     uint32
	iface     int32
	device    uint32
	subdevice uint32
	name      [ctlNameLen]byte
	index     uint32
}

// ctlElemList is struct snd_ctl_elem_list. pids holds a pointer to Go memory
// as a uintptr, so its referent is kept alive across the ioctl by the caller.
type ctlElemList struct {
	offset, space, used, count uint32
	pids                       uintptr
	reserved                   [50]byte
}

// ctlElemInfoValue is the union in struct snd_ctl_elem_info. Its members are
// read through the accessors below rather than declared, because two of them
// hold C longs and so change width between our two architectures. The uint64
// element gives the union the 8-byte alignment its long long member gives it
// in C.
type ctlElemInfoValue struct{ words [16]uint64 }

// ctlElemInfo is struct snd_ctl_elem_info.
type ctlElemInfo struct {
	id       ctlElemID
	typ      int32
	access   uint32
	count    uint32
	owner    int32
	value    ctlElemInfoValue
	reserved [64]byte
}

// ctlEnumInfo is the enumerated arm of ctlElemInfoValue: how many choices the
// element has, which choice ELEM_INFO should name, and the name it wrote back.
type ctlEnumInfo struct {
	items, item uint32
	name        [64]byte
}

// ctlElemValue is struct snd_ctl_elem_value. The value union is
// "long value[128]" at its widest, so it is 1024 bytes on arm64 and 512 on
// armv6 — and since the ioctl number encodes the struct size (see ioctlReq),
// that difference is the difference between two ioctls, not a padding
// nicety.
type ctlElemValue struct {
	id       ctlElemID
	indirect uint32
	_        uint32 // C pads here: the union is 8-aligned by its long long arm
	value    [128]uintptr
	reserved [128]byte
}

// As in the PCM path, these lines fail to *compile* unless each struct is
// exactly the size a C compiler would give it — which means the armv6 layout
// is checked by the same cross-compiles CI already runs.
const (
	wantElemIDSize    = 64
	wantElemListSize  = ((16 + word + 50 + word - 1) / word) * word
	wantElemInfoSize  = 64 + 16 + 128 + 64
	wantElemValueSize = 64 + 8 + 128*word + 128
)

var (
	_ = [1]struct{}{}[unsafe.Sizeof(ctlElemID{})-wantElemIDSize]
	_ = [1]struct{}{}[unsafe.Sizeof(ctlElemList{})-wantElemListSize]
	_ = [1]struct{}{}[unsafe.Sizeof(ctlElemInfo{})-wantElemInfoSize]
	_ = [1]struct{}{}[unsafe.Sizeof(ctlElemValue{})-wantElemValueSize]
)

// ctlMagic is the ioctl type byte for the control interface ('U'), where the
// PCM interface uses 'A'.
const ctlMagic uintptr = 'U'

func ctlReq(dir, size, nr uintptr) uintptr {
	return dir<<30 | size<<16 | ctlMagic<<8 | nr
}

var (
	reqCtlVersion   = ctlReq(iocRead, unsafe.Sizeof(int32(0)), 0x00)
	reqCtlElemList  = ctlReq(iocRead|iocWrite, unsafe.Sizeof(ctlElemList{}), 0x10)
	reqCtlElemInfo  = ctlReq(iocRead|iocWrite, unsafe.Sizeof(ctlElemInfo{}), 0x11)
	reqCtlElemRead  = ctlReq(iocRead|iocWrite, unsafe.Sizeof(ctlElemValue{}), 0x12)
	reqCtlElemWrite = ctlReq(iocRead|iocWrite, unsafe.Sizeof(ctlElemValue{}), 0x13)
)

// maxValues is how many values one element can hold: the value union is
// "long value[128]", so 128 whatever the word width.
const maxValues = 128

func (id *ctlElemID) String() string { return cString(id.name[:]) }

// cString reads a NUL-terminated fixed-width kernel string.
func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// longs reinterprets a value union as the C "long value[128]" it is for
// boolean and integer elements.
func (v *ctlElemValue) longs() *[maxValues]int {
	return (*[maxValues]int)(unsafe.Pointer(&v.value[0]))
}

// items reinterprets a value union as the C "unsigned int item[128]" it is for
// enumerated elements — 32 bits per value on both architectures, unlike longs.
func (v *ctlElemValue) items() *[maxValues]uint32 {
	return (*[maxValues]uint32)(unsafe.Pointer(&v.value[0]))
}

// integerRange reads the "struct { long min, max, step; }" arm of an info
// union.
func (v *ctlElemInfoValue) integerRange() (minVal, maxVal, step int) {
	f := (*[3]int)(unsafe.Pointer(v))
	return f[0], f[1], f[2]
}

// integer64Range reads the "struct { long long min, max, step; }" arm.
func (v *ctlElemInfoValue) integer64Range() (minVal, maxVal, step int) {
	f := (*[3]int64)(unsafe.Pointer(v))
	return int(f[0]), int(f[1]), int(f[2])
}

func (v *ctlElemInfoValue) enum() *ctlEnumInfo {
	return (*ctlEnumInfo)(unsafe.Pointer(v))
}

// control is an open /dev/snd/controlC<card>.
type control struct {
	card int
	file *os.File
}

func openControl(card int) (*control, error) {
	path := fmt.Sprintf("/dev/snd/controlC%d", card)
	file, err := os.OpenFile(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s to reach the card's mixer controls: %w", path, err)
	}
	c := &control{card: card, file: file}
	var version uint32
	if err := c.ioctl(reqCtlVersion, unsafe.Pointer(&version)); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("reading the control protocol version of %s: %w", path, err)
	}
	if version>>16 != ctlVersion>>16 {
		_ = file.Close()
		return nil, fmt.Errorf("%s speaks control protocol %d.%d.%d, this build understands major %d only",
			path, version>>16, (version>>8)&0xff, version&0xff, ctlVersion>>16)
	}
	return c, nil
}

func (c *control) Close() error { return c.file.Close() }

func (c *control) ioctl(req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, c.file.Fd(), req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

// ids lists every element id on the card. The first ELEM_LIST asks for no
// space at all, which the kernel answers with the total count; the second one
// fills a buffer that size.
func (c *control) ids() ([]ctlElemID, error) {
	var list ctlElemList
	if err := c.ioctl(reqCtlElemList, unsafe.Pointer(&list)); err != nil {
		return nil, fmt.Errorf("counting the control elements of card %d: %w", c.card, err)
	}
	if list.count == 0 {
		return nil, nil
	}
	ids := make([]ctlElemID, list.count)
	list = ctlElemList{space: uint32(len(ids)), pids: uintptr(unsafe.Pointer(&ids[0]))}
	err := c.ioctl(reqCtlElemList, unsafe.Pointer(&list))
	runtime.KeepAlive(ids)
	if err != nil {
		return nil, fmt.Errorf("listing the control elements of card %d: %w", c.card, err)
	}
	return ids[:min(int(list.used), len(ids))], nil
}

// info describes one element. The kernel resolves it by numid and fills in the
// rest of the id, so the name comes from here rather than from the list.
func (c *control) info(numid uint32) (*ctlElemInfo, error) {
	in := &ctlElemInfo{id: ctlElemID{numid: numid}}
	if err := c.ioctl(reqCtlElemInfo, unsafe.Pointer(in)); err != nil {
		return nil, fmt.Errorf("describing control element %d of card %d: %w", numid, c.card, err)
	}
	return in, nil
}

func (c *control) read(numid uint32) (*ctlElemValue, error) {
	v := &ctlElemValue{id: ctlElemID{numid: numid}}
	if err := c.ioctl(reqCtlElemRead, unsafe.Pointer(v)); err != nil {
		return nil, fmt.Errorf("reading control element %d of card %d: %w", numid, c.card, err)
	}
	return v, nil
}

// write sets an element's values. Enumerated elements take item indices, which
// the ABI stores as 32-bit ints rather than as the longs everything else uses.
func (c *control) write(numid uint32, typ ControlType, values []int) error {
	if len(values) > maxValues {
		return fmt.Errorf("control element %d of card %d: %d values is more than the ABI's %d",
			numid, c.card, len(values), maxValues)
	}
	v := &ctlElemValue{id: ctlElemID{numid: numid}}
	for i, n := range values {
		if typ == ControlEnumerated {
			v.items()[i] = uint32(n)
		} else {
			v.longs()[i] = n
		}
	}
	if err := c.ioctl(reqCtlElemWrite, unsafe.Pointer(v)); err != nil {
		return fmt.Errorf("setting control element %d of card %d to %v: %w", numid, c.card, values, err)
	}
	return nil
}

// elements reads every control element on the card, with its current value.
func (c *control) elements() ([]Control, error) {
	ids, err := c.ids()
	if err != nil {
		return nil, err
	}
	out := make([]Control, 0, len(ids))
	for _, id := range ids {
		e, err := c.element(id.numid)
		if err != nil {
			return out, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (c *control) element(numid uint32) (Control, error) {
	in, err := c.info(numid)
	if err != nil {
		return Control{}, err
	}
	e := Control{
		Numid:    int(in.id.numid),
		Iface:    Iface(in.id.iface),
		Name:     cString(in.id.name[:]),
		Index:    int(in.id.index),
		Type:     ControlType(in.typ),
		Readable: in.access&ctlAccessRead != 0,
		Writable: in.access&ctlAccessWrite != 0,
		Inactive: in.access&ctlAccessInactive != 0,
		Max:      1,
		Step:     1,
	}
	switch e.Type {
	case ControlInteger:
		e.Min, e.Max, e.Step = in.value.integerRange()
	case ControlInteger64:
		e.Min, e.Max, e.Step = in.value.integer64Range()
	case ControlEnumerated:
		e.Items, err = c.enumItems(numid, int(in.value.enum().items))
		if err != nil {
			return e, err
		}
		e.Max = max(len(e.Items)-1, 0)
	}
	if !e.Readable || e.Type == ControlBytes || e.Type == ControlIEC958 {
		return e, nil
	}
	e.Values, err = c.values(numid, e.Type, min(int(in.count), maxValues))
	return e, err
}

// enumItems names an enumerated element's choices, which takes one ELEM_INFO
// per choice: the kernel writes the name of whichever item the caller asks
// about back into the same struct.
func (c *control) enumItems(numid uint32, items int) ([]string, error) {
	out := make([]string, 0, items)
	for i := 0; i < items; i++ {
		in := &ctlElemInfo{id: ctlElemID{numid: numid}}
		in.value.enum().item = uint32(i)
		if err := c.ioctl(reqCtlElemInfo, unsafe.Pointer(in)); err != nil {
			return out, fmt.Errorf("naming choice %d of control element %d on card %d: %w", i, numid, c.card, err)
		}
		out = append(out, cString(in.value.enum().name[:]))
	}
	return out, nil
}

func (c *control) values(numid uint32, typ ControlType, count int) ([]int, error) {
	v, err := c.read(numid)
	if err != nil {
		return nil, err
	}
	out := make([]int, count)
	for i := range out {
		if typ == ControlEnumerated {
			out[i] = int(v.items()[i])
		} else {
			out[i] = v.longs()[i]
		}
	}
	return out, nil
}

// The SNDRV_CTL_ELEM_ACCESS_* bits this package reads.
const (
	ctlAccessRead     = 1 << 0
	ctlAccessWrite    = 1 << 1
	ctlAccessInactive = 1 << 8
)

// applyAudibility runs the audibility pass against a card: read every control
// element, decide what has to change for a stream to be heard, and write
// every one of those changes — a control this codec doesn't have, or a
// transient DAPM power race, does not stop the rest from being attempted.
// It reports every change that succeeded, plus every failure joined into one
// error, so a caller can log how far it got.
func applyAudibility(card int, volume int, prefer Output) ([]Change, error) {
	c, err := openControl(card)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()

	elements, err := c.elements()
	if err != nil {
		return nil, err
	}
	byNumid := make(map[int]ControlType, len(elements))
	for _, e := range elements {
		byNumid[e.Numid] = e.Type
	}
	return applyChanges(c, byNumid, audibilityPass(elements, volume, prefer))
}
