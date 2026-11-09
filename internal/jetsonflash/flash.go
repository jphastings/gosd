// Package jetsonflash flashes a gosd image onto a Jetson Nano 16GB eMMC
// module waiting in USB recovery mode, without NVIDIA's host tools (bean
// gosd-32dw). It boots the module into CBoot's flashing server with the boot
// chain internal/l4t builds from NVIDIA's L4T kit, then writes every
// partition of a first flash, with the image's boot partition as APP.
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
	"time"

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

// RunFlash writes a gosd image to a Jetson Nano eMMC module in USB recovery
// mode, erasing everything on its eMMC. Everything that can fail without the
// module is checked before it is touched. Progress goes to out.
func RunFlash(imagePath string, o FlashOptions, out io.Writer) (err error) {
	img, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, img.Close()) }()
	app, err := bootPartition(img)
	if err != nil {
		return fmt.Errorf("%s isn't a gosd image: %w; build one with `gosd build --board jetson-nano`", imagePath, err)
	}
	if app.Size() > l4t.AppSize {
		return fmt.Errorf("%s's boot partition is %d MiB, too big for the module's %d MiB APP partition; build it with a smaller --boot-size", imagePath, app.Size()>>20, l4t.AppSize>>20)
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

// bootPartition returns MBR partition 1 of a disk image as a reader.
func bootPartition(img io.ReaderAt) (*io.SectionReader, error) {
	mbr := make([]byte, sectorSize)
	if _, err := img.ReadAt(mbr, 0); err != nil {
		return nil, fmt.Errorf("reading its first sector: %w", err)
	}
	if mbr[510] != 0x55 || mbr[511] != 0xaa {
		return nil, errors.New("it has no MBR")
	}
	// internal/image writes exactly this type for the boot partition.
	if typ := mbr[0x1be+4]; typ != 0x0c {
		return nil, fmt.Errorf("its first partition has type %#02x, not a gosd boot partition's FAT32 (0x0c)", typ)
	}
	start := int64(binary.LittleEndian.Uint32(mbr[0x1be+8:]))
	count := int64(binary.LittleEndian.Uint32(mbr[0x1be+12:]))
	if count == 0 {
		return nil, errors.New("its first partition is empty")
	}
	if _, err := img.ReadAt(make([]byte, 1), (start+count)*sectorSize-1); err != nil {
		return nil, errors.New("it ends before its first partition does, so it's probably truncated")
	}
	return io.NewSectionReader(img, start*sectorSize, count*sectorSize), nil
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
