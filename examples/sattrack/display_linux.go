//go:build linux

package main

import (
	"errors"
	"fmt"
	"image"
	"os"
	"runtime"
	"unsafe"

	"github.com/NeowayLabs/drm"
	"github.com/NeowayLabs/drm/mode"
	"golang.org/x/sys/unix"
)

// display owns one DRM legacy-modeset pipeline: the first connected
// connector's preferred mode on the first usable card, scanning out a
// single dumb XRGB8888 framebuffer that Flush copies dirty backbuffer
// rectangles into.
type display struct {
	file    *os.File
	fbID    uint32
	handle  uint32
	pitch   int
	fbMem   []byte
	size    image.Point
	crtcID  uint32
	connID  uint32
	modeSet mode.Info

	dirtyFBUnsupported bool
}

// maxCards is how many /dev/dri/cardN nodes openDisplay probes before
// giving up; KMS cards essentially always enumerate from 0.
const maxCards = 4

// openDisplay finds a KMS card with dumb-buffer support and a connected
// connector, then performs the classic legacy modeset: preferred mode ->
// dumb buffer -> AddFB -> mmap -> SetCrtc.
func openDisplay() (*display, error) {
	var lastErr error
	for n := 0; n < maxCards; n++ {
		file, err := drm.OpenCard(n)
		if err != nil {
			if lastErr == nil {
				lastErr = fmt.Errorf("opening /dev/dri/card%d: %w", n, err)
			}
			continue
		}
		d, err := setupCard(file)
		if err != nil {
			lastErr = err
			_ = file.Close()
			continue
		}
		return d, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no /dev/dri/card* device found")
	}
	return nil, lastErr
}

func setupCard(file *os.File) (*display, error) {
	if !drm.HasDumbBuffer(file) {
		return nil, fmt.Errorf("%s has no dumb-buffer support", file.Name())
	}
	res, err := mode.GetResources(file)
	if err != nil {
		return nil, fmt.Errorf("reading %s's mode resources: %w", file.Name(), err)
	}

	for _, connID := range res.Connectors {
		conn, err := mode.GetConnector(file, connID)
		if err != nil || conn.Connection != mode.Connected || len(conn.Modes) == 0 {
			continue
		}
		crtcID, err := findCrtc(file, res, conn)
		if err != nil {
			continue
		}
		return newFramebuffer(file, conn, crtcID)
	}
	return nil, fmt.Errorf("%s has no connected connector", file.Name())
}

// findCrtc resolves a CRTC for conn: its current encoder's CRTC when it
// has one, otherwise the first CRTC any of its encoders can drive.
func findCrtc(file *os.File, res *mode.Resources, conn *mode.Connector) (uint32, error) {
	if conn.EncoderID != 0 {
		if enc, err := mode.GetEncoder(file, conn.EncoderID); err == nil && enc.CrtcID != 0 {
			return enc.CrtcID, nil
		}
	}
	for _, encID := range conn.Encoders {
		enc, err := mode.GetEncoder(file, encID)
		if err != nil {
			continue
		}
		for i, crtcID := range res.Crtcs {
			if enc.PossibleCrtcs&(1<<uint(i)) != 0 {
				return crtcID, nil
			}
		}
	}
	return 0, fmt.Errorf("no CRTC available for connector %d", conn.ID)
}

func newFramebuffer(file *os.File, conn *mode.Connector, crtcID uint32) (*display, error) {
	m := conn.Modes[0] // the kernel sorts the preferred mode first
	fb, err := mode.CreateFB(file, m.Hdisplay, m.Vdisplay, 32)
	if err != nil {
		return nil, fmt.Errorf("creating a %dx%d dumb framebuffer: %w", m.Hdisplay, m.Vdisplay, err)
	}
	fbID, err := mode.AddFB(file, m.Hdisplay, m.Vdisplay, 24, 32, fb.Pitch, fb.Handle)
	if err != nil {
		return nil, fmt.Errorf("adding the framebuffer: %w", err)
	}
	offset, err := mode.MapDumb(file, fb.Handle)
	if err != nil {
		return nil, fmt.Errorf("mapping the dumb buffer: %w", err)
	}
	mem, err := unix.Mmap(int(file.Fd()), int64(offset), int(fb.Size),
		unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmapping the framebuffer: %w", err)
	}

	d := &display{
		file:    file,
		fbID:    fbID,
		handle:  fb.Handle,
		pitch:   int(fb.Pitch),
		fbMem:   mem,
		size:    image.Pt(int(m.Hdisplay), int(m.Vdisplay)),
		crtcID:  crtcID,
		connID:  conn.ID,
		modeSet: m,
	}
	if err := mode.SetCrtc(file, crtcID, fbID, 0, 0, &d.connID, 1, &d.modeSet); err != nil {
		_ = unix.Munmap(mem)
		return nil, fmt.Errorf("setting the CRTC mode: %w", err)
	}
	return d, nil
}

// Size is the mode's active area.
func (d *display) Size() image.Point { return d.size }

// Flush copies each dirty rect from the RGBA backbuffer into the scanout
// buffer (XRGB8888 little-endian: B,G,R,X in memory, rows d.pitch bytes
// apart) and reports the rects as damage clips. virtio-gpu only updates
// the host window on damage; real scanout hardware (vc4) ignores it.
func (d *display) Flush(buf *image.RGBA, rects []image.Rectangle) error {
	bounds := image.Rectangle{Max: d.size}
	for _, r := range rects {
		r = r.Intersect(bounds)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			src := buf.Pix[buf.PixOffset(r.Min.X, y):buf.PixOffset(r.Max.X, y)]
			dst := d.fbMem[y*d.pitch+r.Min.X*4:]
			for i := 0; i+3 < len(src); i += 4 {
				dst[i] = src[i+2]   // B
				dst[i+1] = src[i+1] // G
				dst[i+2] = src[i]   // R
				dst[i+3] = 0
			}
		}
	}
	return d.markDirty(rects)
}

// drmClipRect and drmModeFBDirtyCmd mirror the kernel's struct
// drm_clip_rect and struct drm_mode_fb_dirty_cmd (drm_mode.h)
// field-for-field; NeowayLabs/drm predates DIRTYFB support so the ioctl is
// carried here.
type drmClipRect struct {
	x1, y1, x2, y2 uint16
}

type drmModeFBDirtyCmd struct {
	fbID     uint32
	flags    uint32
	color    uint32
	numClips uint32
	clipsPtr uint64
}

// ioctlModeDirtyFB is DRM_IOWR(0xB1, struct drm_mode_fb_dirty_cmd) - see
// include/uapi/drm/drm.h's DRM_IOCTL_MODE_DIRTYFB (0xB1; neighboring 0xB5
// is GETPLANERESOURCES, whose identical struct size makes a mixed-up nr
// "succeed" silently while no damage ever reaches the device).
var ioctlModeDirtyFB = iowr('d', 0xb1, unsafe.Sizeof(drmModeFBDirtyCmd{}))

func iowr(typ byte, nr uint, size uintptr) uintptr {
	const (
		nrShift   = 0
		typeShift = 8
		sizeShift = 16
		dirShift  = 30
		dirRead   = uint32(2)
		dirWrite  = uint32(1)
	)
	return uintptr((dirRead|dirWrite)<<dirShift | uint32(size)<<sizeShift | uint32(typ)<<typeShift | uint32(nr)<<nrShift)
}

// markDirty issues DRM_IOCTL_MODE_DIRTYFB for the flushed rects. Drivers
// for hardware that scans out continuously (vc4) report the ioctl as
// unsupported - that's success, and the ioctl is skipped from then on.
func (d *display) markDirty(rects []image.Rectangle) error {
	if d.dirtyFBUnsupported || len(rects) == 0 {
		return nil
	}

	clips := make([]drmClipRect, 0, len(rects))
	bounds := image.Rectangle{Max: d.size}
	for _, r := range rects {
		r = r.Intersect(bounds)
		if r.Empty() {
			continue
		}
		clips = append(clips, drmClipRect{
			x1: uint16(r.Min.X), y1: uint16(r.Min.Y),
			x2: uint16(r.Max.X), y2: uint16(r.Max.Y),
		})
	}
	if len(clips) == 0 {
		return nil
	}

	cmd := drmModeFBDirtyCmd{
		fbID:     d.fbID,
		numClips: uint32(len(clips)),
		clipsPtr: uint64(uintptr(unsafe.Pointer(&clips[0]))),
	}
	//nolint:staticcheck // SA1019: unix.SYS_IOCTL is fine here; this file is linux-only.
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, d.file.Fd(), ioctlModeDirtyFB, uintptr(unsafe.Pointer(&cmd)))
	runtime.KeepAlive(clips)
	switch errno {
	case 0:
		return nil
	case unix.ENOSYS, unix.EOPNOTSUPP, unix.EINVAL:
		// The driver doesn't do damage tracking; it doesn't need to.
		d.dirtyFBUnsupported = true
		return nil
	default:
		return fmt.Errorf("DRM_IOCTL_MODE_DIRTYFB failed: %w", errno)
	}
}

// Close releases the modeset resources (the process exiting would too;
// this keeps the retry loop tidy).
func (d *display) Close() error {
	_ = unix.Munmap(d.fbMem)
	_ = mode.RmFB(d.file, d.fbID)
	_ = mode.DestroyDumb(d.file, d.handle)
	return d.file.Close()
}
