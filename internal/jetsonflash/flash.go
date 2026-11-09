// Package jetsonflash flashes a gosd image onto a Jetson Nano 16GB eMMC
// module waiting in USB recovery mode, without NVIDIA's host tools (bean
// gosd-32dw). It boots the module into CBoot's flashing server with the boot
// chain internal/l4t builds from NVIDIA's L4T kit, then writes every
// partition of a first flash, with the image's root or boot filesystem as
// APP.
package jetsonflash

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/jphastings/gosd/internal/l4t"
)

const (
	sectorSize    = 512
	appPartition  = "APP"
	nameFieldSize = 40
	// writeSettle is how long CBoot may take to report a finished write or
	// flush.
	writeSettle = 30 * time.Second
)

// FlashOptions say where NVIDIA's kit comes from and how to report.
type FlashOptions struct {
	// Kit is a copy of NVIDIA's L4T kit to read instead of downloading it.
	Kit string
	// AcceptLicense accepts NVIDIA's licence for the kit without asking.
	AcceptLicense bool
	// Terminal is where to ask the user to accept NVIDIA's licence, or nil
	// if there is no one to ask.
	Terminal io.Reader
	// Trace, if non-nil, receives every USB transfer and nv3p packet.
	Trace io.Writer
}

// RunFlash writes an image to a Jetson Nano eMMC module in USB recovery
// mode, erasing everything on its eMMC. Everything that can fail without the
// module is checked before it is touched. Progress goes to out.
func RunFlash(imagePath string, o FlashOptions, out io.Writer) (err error) {
	img, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, img.Close()) }()
	info, err := img.Stat()
	if err != nil {
		return err
	}
	app, err := bootPartition(img, info.Size())
	if err != nil {
		return fmt.Errorf("can't flash %s: %w; give it a GPT or MBR disk image, or a bare ext4 or FAT filesystem image", imagePath, err)
	}
	if app.Size() > l4t.AppSize {
		return fmt.Errorf("can't flash %s: the filesystem it would write as APP is %d MiB, too big for the module's %d MiB APP partition; shrink that filesystem and its partition", imagePath, app.Size()>>20, l4t.AppSize>>20)
	}
	if err := hostError(); err != nil {
		return err
	}

	in, err := kitInputs(l4t.R32_7_6, cacheRoot(), o, out)
	if err != nil {
		return err
	}
	bc, err := l4t.Build(in)
	if err != nil {
		return err
	}
	// The GUIDs and the version file's time are drawn once, so that every
	// plan for the same eMMC is the same, byte for byte.
	seed := make([]byte, 4096) // ponytail: room for 256 GUIDs; R32.7.6's layout draws 40
	if _, err := rand.Read(seed); err != nil {
		return err
	}
	now := time.Now()
	plan := func(s l4t.Storage) (l4t.FirstFlash, error) {
		return l4t.BuildFirstFlash(in, s, now, bytes.NewReader(seed))
	}

	log := io.Discard
	if o.Trace != nil {
		log = o.Trace
	}
	start := time.Now()
	open := func() (conn, error) {
		c, err := openRecovery(deviceWait)
		if err != nil {
			return nil, err
		}
		if o.Trace == nil {
			return c, nil
		}
		return tracedConn{conn: c, w: o.Trace, start: start}, nil
	}
	return flash(open, bc, plan, app, out, log)
}

// probeSize covers every signature bootPartition reads: the GPT header, a
// FAT boot sector and an ext4 superblock.
const probeSize = 2048

// bootPartition returns the part of an image that becomes APP, which NVIDIA's
// U-Boot scans for extlinux.conf: the partition named APP of a GPT disk
// image, otherwise its first; partition 1 of an MBR disk image; or the whole
// of a bare ext4 or FAT filesystem image. The image's own bytes say which
// it is. The chosen region must hold FAT or ext4, the only filesystems
// U-Boot reads.
func bootPartition(img io.ReaderAt, imgSize int64) (*io.SectionReader, error) {
	head, err := sectorsAt(img, 0)
	if err != nil {
		return nil, fmt.Errorf("reading its first sectors: %w", err)
	}
	var start, size int64
	switch {
	case string(head[sectorSize:sectorSize+8]) == "EFI PART": // before ext4: its entries start at byte 1024
		start, size, err = gptApp(img, head[sectorSize:])
	case isFAT(head): // before MBR: a FAT boot sector also ends in 0x55aa
		size = imgSize
	case head[510] == 0x55 && head[511] == 0xaa:
		start = int64(binary.LittleEndian.Uint32(head[0x1be+8:])) * sectorSize
		size = int64(binary.LittleEndian.Uint32(head[0x1be+12:])) * sectorSize
		if size == 0 {
			return nil, errors.New("its first partition is empty")
		}
	case isExt4(head):
		size = imgSize
	default:
		return nil, errors.New("it has no GPT, no MBR and no ext4 or FAT filesystem at its start")
	}
	if err != nil {
		return nil, err
	}
	if start+size > imgSize {
		return nil, errors.New("it ends before its partition does, so it's probably truncated")
	}
	fs, err := sectorsAt(img, start)
	if err != nil {
		return nil, fmt.Errorf("reading the partition it would write as APP: %w", err)
	}
	if !isFAT(fs) && !isExt4(fs) {
		return nil, errors.New("the partition it would write as APP holds neither FAT nor ext4, the only filesystems NVIDIA's U-Boot reads")
	}
	return io.NewSectionReader(img, start, size), nil
}

// sectorsAt reads probeSize bytes at off, zero-filled past the image's end.
func sectorsAt(img io.ReaderAt, off int64) ([]byte, error) {
	b := make([]byte, probeSize)
	if _, err := img.ReadAt(b, off); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b, nil
}

func isFAT(b []byte) bool {
	return string(b[0x52:0x5a]) == "FAT32   " || string(b[0x36:0x39]) == "FAT"
}

func isExt4(b []byte) bool { return binary.LittleEndian.Uint16(b[1080:]) == 0xef53 }

// gptApp finds the byte range of the partition named APP in a GPT whose
// header is hdr, or else of its first partition.
func gptApp(img io.ReaderAt, hdr []byte) (start, size int64, err error) {
	entriesAt := int64(binary.LittleEndian.Uint64(hdr[72:]))
	count := int64(binary.LittleEndian.Uint32(hdr[80:]))
	entrySize := int64(binary.LittleEndian.Uint32(hdr[84:]))
	if entrySize < 128 || count*entrySize > 1<<20 || entriesAt>>40 != 0 {
		return 0, 0, errors.New("its GPT is malformed")
	}
	entries := make([]byte, count*entrySize)
	if _, err := img.ReadAt(entries, entriesAt*sectorSize); err != nil {
		return 0, 0, errors.New("it ends before its GPT's partition entries do, so it's probably truncated")
	}
	var chosen []byte
	for i := int64(0); i < count; i++ {
		e := entries[i*entrySize : (i+1)*entrySize]
		if !slices.ContainsFunc(e[:16], func(b byte) bool { return b != 0 }) {
			continue
		}
		if chosen == nil {
			chosen = e
		}
		if gptName(e[56:128]) == appPartition {
			chosen = e
			break
		}
	}
	if chosen == nil {
		return 0, 0, errors.New("its GPT has no partitions")
	}
	first, last := binary.LittleEndian.Uint64(chosen[32:]), binary.LittleEndian.Uint64(chosen[40:])
	if last < first || last>>40 != 0 {
		return 0, 0, errors.New("its GPT has a partition with an impossible size")
	}
	return int64(first) * sectorSize, int64(last-first+1) * sectorSize, nil
}

func gptName(b []byte) string {
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return strings.TrimRight(string(utf16.Decode(units)), "\x00")
}

// p3448Storage is the Jetson Nano 16GB eMMC module's (P3448-0002) eMMC as
// CBoot's storage query reported it on the bench (node 3, 2026-11-08): the
// boot device (boot0 and boot1), then the user area, in 512-byte sectors.
var p3448Storage = l4t.Storage{BootSectors: 16384, UserSectors: 30777344}

// flash boots the module into CBoot's flashing server, reads the eMMC's
// size, and writes plan's first flash for it with app as APP: GPT first,
// then every partition in NVIDIA's order, then a flush, then the BCT. Each
// write and flush is a session of its own and must end in CBoot's STATUS 0
// before the next one starts. Then it flushes again and cold-boots.
//
// The server finds the boot device's partitions through the BCT downloaded
// in recovery mode, so, as in NVIDIA's flash, that is the BCT written last.
// It has to be built before the eMMC's size can be read, so it is built for
// p3448Storage's, and the plan is rebuilt for the size the query returns.
// plan is deterministic, and the BCT doesn't depend on the user area's
// size, so the two BCTs match unless the boot device differs; then nothing
// is written.
//
// Crash ordering. Recovery from an interrupted flash rests on two things:
//   - The recovery strap puts the module into the boot ROM's USB recovery
//     mode, which is in mask ROM, whatever the eMMC holds. Nothing
//     jetson-flash sends can change that: it sends no fuse commands, and no
//     OEM command but type 1.
//   - Every run is a whole first flash from scratch, and depends on nothing
//     already on the eMMC.
//
// So whatever an interrupted flash leaves behind, strapping the module and
// running `jetson-flash flash` again recovers it.
//
// Observed on the bench, not guaranteed (2026-11-09): with no BCT, the boot
// ROM falls back to USB recovery mode by itself. A flash stopped just after
// the GPT write, then a plain power-on with the strap released, brought up
// the recovery device with nothing on the console; the same power-on after
// a complete flash booted the app. Still unobserved, so the strap is their
// backstop: a power-on part-way through the boot device's writes, a
// boot-device erase cut short (which may leave an old BCT copy), and a BCT
// torn part-way.
//
// Writing the BCT last, after every other write is confirmed and flushed,
// then flushing it too, is a best effort so that a plain power-on doesn't
// run a half-written boot chain; recovery doesn't depend on it. On the
// bench, CBoot took the BCT write after the first flush, and both flushes
// ended in a STATUS.
func flash(open func() (conn, error), bc l4t.BootChain, plan func(l4t.Storage) (l4t.FirstFlash, error), app *io.SectionReader, out, log io.Writer) error {
	booted, err := plan(p3448Storage)
	if err != nil {
		return err
	}
	if err := bootToServer(open, bc, booted.BCT, out, log); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "== reading the eMMC's size")
	var storage l4t.Storage
	err = inSession(open, log, func(s *session) (err error) {
		storage, err = s.storage()
		return err
	})
	if err != nil {
		return fmt.Errorf("reading the eMMC's size: %w", err)
	}
	_, _ = fmt.Fprintf(out, "   boot device %d sectors, user area %d\n", storage.BootSectors, storage.UserSectors)
	ff, err := plan(storage)
	if err != nil {
		return err
	}
	if !bytes.Equal(ff.BCT, booted.BCT) {
		return fmt.Errorf("the module's eMMC has a %d-sector boot device and a %d-sector user area, which need a different BCT from the one it was booted with, built for the Jetson Nano 16GB eMMC module's %d and %d; nothing has been written, so check that module (P3448-0002) is the one attached", storage.BootSectors, storage.UserSectors, p3448Storage.BootSectors, p3448Storage.UserSectors)
	}

	write := func(p l4t.Partition) error {
		var data io.Reader = bytes.NewReader(p.Data)
		size := int64(len(p.Data))
		switch p.Name {
		case "GPT":
			_, _ = fmt.Fprintln(out, "== writing GPT, which erases the eMMC")
		case appPartition:
			_, _ = fmt.Fprintf(out, "== writing %s (%d MiB)\n", p.Name, app.Size()>>20)
			data, size = &progress{r: app, total: app.Size(), out: out}, app.Size()
		default:
			_, _ = fmt.Fprintf(out, "== writing %s\n", p.Name)
		}
		if err := inSession(open, log, func(s *session) error { return s.writePartition(p.Name, data, size) }); err != nil {
			return fmt.Errorf("writing %s: %w", p.Name, err)
		}
		return nil
	}
	flush := func() error {
		_, _ = fmt.Fprintln(out, "== flushing to storage")
		err := inSession(open, log, func(s *session) error {
			if err := s.command(cmdFlush, nil); err != nil {
				return err
			}
			return s.awaitStatus(writeSettle)
		})
		if err != nil {
			return fmt.Errorf("flushing to storage: %w", err)
		}
		return nil
	}

	partlyWritten := func(err error) error {
		return fmt.Errorf("%w; the module's eMMC may now be partly written, so it may not boot: put it back into recovery mode by power-cycling it with its recovery strap held (on a Turing Pi 2: tpi usb flash -n N --bmc, tpi power -n N off, wait 10 s, tpi power -n N on) and run jetson-flash flash again", err)
	}
	for _, p := range slices.Concat([]l4t.Partition{{Name: "GPT", Data: ff.GPT}}, ff.Partitions) {
		if err := write(p); err != nil {
			return partlyWritten(err)
		}
	}
	if err := flush(); err != nil {
		return partlyWritten(err)
	}
	if err := write(l4t.Partition{Name: "BCT", Data: ff.BCT}); err != nil {
		return partlyWritten(err)
	}
	if err := flush(); err != nil {
		return partlyWritten(err)
	}

	_, _ = fmt.Fprintln(out, "== restarting the module")
	err = inSession(open, log, func(s *session) error {
		// The device resets as soon as it has acknowledged the command, so
		// whatever fails after the ACK is the reset itself.
		if err := s.command(cmdColdBoot, le32s(0)); err != nil {
			return err
		}
		_ = s.drain(quietWait)
		return nil
	})
	if err != nil {
		return fmt.Errorf("restarting the module: %w; everything is written and flushed, so release the recovery strap and power-cycle the module to boot the new image", err)
	}
	_, _ = fmt.Fprintln(out, "done: the module is booting the new image")
	return nil
}

// storage asks CBoot's flashing server for the eMMC's size with OEM type 1
// (platform details), sub-command 1 (storage), in the whole 48-byte argument
// buffer. It replies {u64 0; u32 sectors; u32 sector size} for the boot
// device, then the same for the user area.
func (s *session) storage() (l4t.Storage, error) {
	var reply []byte
	s.onData = func(b []byte) { reply = append(reply, b...) }
	args := make([]byte, 48)
	args[0], args[4] = 1, 1
	if err := s.command(cmdOEM, args); err != nil {
		return l4t.Storage{}, err
	}
	if err := s.drain(replyTimeout); err != nil {
		return l4t.Storage{}, err
	}
	if len(reply) != 32 || le32(reply[12:]) != sectorSize || le32(reply[28:]) != sectorSize {
		return l4t.Storage{}, fmt.Errorf("CBoot described the eMMC as % x, not as two devices of 512-byte sectors", reply)
	}
	return l4t.Storage{BootSectors: int64(le32(reply[8:])), UserSectors: int64(le32(reply[24:]))}, nil
}

// writePartition sends CBoot's flashing server its {char name[40]; u64 size}
// command (any longer hangs the server until a power cycle), then size
// bytes of data, then waits for the STATUS confirming the write.
func (s *session) writePartition(name string, data io.Reader, size int64) error {
	args := make([]byte, nameFieldSize, nameFieldSize+8)
	copy(args, name)
	args = binary.LittleEndian.AppendUint64(args, uint64(size))
	if err := s.command(cmdWritePartition, args); err != nil {
		return err
	}
	if err := s.drain(quietWait); err != nil {
		return err
	}
	if err := s.stream(data); err != nil {
		return err
	}
	return s.awaitStatus(writeSettle)
}

// awaitStatus is drain for a command that must end in a STATUS: silence
// until timeout is a failure, not the device having nothing to say.
func (s *session) awaitStatus(timeout time.Duration) error {
	before := s.statuses
	if err := s.drain(timeout); err != nil {
		return err
	}
	if s.statuses == before {
		return fmt.Errorf("CBoot didn't confirm it within %s", timeout)
	}
	return nil
}
