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

	in, err := kitInputs(l4t.R32_7_6, cacheRoot(), o, out)
	if err != nil {
		return err
	}
	bc, err := l4t.Build(in)
	if err != nil {
		return err
	}
	plan := func(s l4t.Storage) (l4t.FirstFlash, error) {
		return l4t.BuildFirstFlash(in, s, time.Now(), rand.Reader)
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

// flash boots the module into CBoot's flashing server, reads the eMMC's
// size, and writes plan's first flash for it with app as APP: GPT first,
// then every partition in NVIDIA's order, then a flush, then the BCT. Each
// write and flush is a session of its own and must end in CBoot's STATUS 0
// before the next one starts. Then it flushes again and cold-boots.
//
// Crash ordering: every flash is a whole first flash, so an interrupted one
// is recovered by flashing again from the start. The order only has to keep
// a half-written eMMC from booting:
//   - The GPT write erases the boot device, where the BCT lives, before it
//     sanitizes the user area and writes the GPTs. From then on the eMMC
//     has no BCT.
//   - Every other write is confirmed by CBoot and then flushed before the
//     BCT is written, so the BCT can't reach the eMMC ahead of anything it
//     describes, however CBoot buffers writes.
//   - The BCT is flushed in turn. Until it lands the eMMC has no BCT, and the
//     boot ROM boots nothing from it. A BCT cut short fails its own CMAC,
//     which the boot ROM presumably treats the same way.
//   - Recovery mode belongs to the boot ROM, which enters it on the
//     recovery strap, so `jetson-flash flash` can always start over.
//
// All of that is observed on the bench (2026-11-08), not documented by
// NVIDIA. CBoot's console showed the erase order. A blank module showed
// nothing on power-on, and entered recovery mode on the strap. We haven't
// seen the boot ROM fall back to recovery mode by itself, and nothing
// relies on it. The flush before the BCT is pending bench confirmation:
// CBoot hasn't yet been seen to take a write after a flush.
func flash(open func() (conn, error), bc l4t.BootChain, plan func(l4t.Storage) (l4t.FirstFlash, error), app *io.SectionReader, out, log io.Writer) error {
	if err := bootToServer(open, bc, out, log); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "== reading the eMMC's size")
	var storage l4t.Storage
	err := inSession(open, log, func(s *session) (err error) {
		storage, err = s.storage()
		return err
	})
	if err != nil {
		return fmt.Errorf("reading the eMMC's size: %w", err)
	}
	ff, err := plan(storage)
	if err != nil {
		return err
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

	for _, p := range slices.Concat([]l4t.Partition{{Name: "GPT", Data: ff.GPT}}, ff.Partitions) {
		if err := write(p); err != nil {
			return err
		}
	}
	if err := flush(); err != nil {
		return err
	}
	if err := write(l4t.Partition{Name: "BCT", Data: ff.BCT}); err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
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
		return fmt.Errorf("restarting the module: %w", err)
	}
	_, _ = fmt.Fprintln(out, "done: the module is booting the new image")
	return nil
}

// oemStorageQuery is the whole 48-byte argument buffer of the only OEM
// command jetson-flash sends: type 1 (platform details), sub-command 1
// (storage). Never send another type: type 9 provisions the eMMC's RPMB
// key, which can never be undone (bench, 2026-11-08).
var oemStorageQuery = [48]byte{0: 1, 4: 1}

// storage asks CBoot's flashing server for the eMMC's size. It replies
// {u64 0; u32 sectors; u32 sector size} for the boot device, then the same
// for the user area.
func (s *session) storage() (l4t.Storage, error) {
	var reply []byte
	s.onData = func(b []byte) { reply = append(reply, b...) }
	if err := s.command(cmdOEM, oemStorageQuery[:]); err != nil {
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
