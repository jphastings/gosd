package jetsonflash

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	sectorSize    = 512
	appPartition  = "APP"
	nameFieldSize = 40
	// writeSettle is how long CBoot may take to report a finished write or
	// flush before we carry on regardless.
	writeSettle = 30 * time.Second
)

// RunFlash rewrites the APP partition of a module waiting in USB recovery
// mode with the boot partition of a gosd image, using the NVIDIA-signed
// files in dir (an unpacked massflash blob). The module must already carry
// NVIDIA's partition layout. Progress goes to out; trace, if non-nil,
// receives every USB transfer and packet.
func RunFlash(imagePath, dir string, out, trace io.Writer) (err error) {
	img, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, img.Close()) }()
	boot, err := bootPartition(img)
	if err != nil {
		return fmt.Errorf("%s isn't a gosd image: %w; build one with `gosd build --board jetson-nano`", imagePath, err)
	}
	f, err := loadSpikeFiles(dir)
	if err != nil {
		return err
	}

	log := io.Discard
	if trace != nil {
		log = trace
	}
	start := time.Now()
	open := func() (conn, error) {
		c, err := openRecovery(deviceWait)
		if err != nil {
			return nil, err
		}
		if trace == nil {
			return c, nil
		}
		return tracedConn{conn: c, w: trace, start: start}, nil
	}
	return flash(open, f, boot, out, log)
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
	return io.NewSectionReader(img, start*sectorSize, count*sectorSize), nil
}

func flash(open func() (conn, error), f spikeFiles, boot *io.SectionReader, out, log io.Writer) error {
	if err := bootToServer(open, f, out, log); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "== writing %s (%d MiB)\n", appPartition, boot.Size()>>20)
	err := inSession(open, log, func(s *session) error { return s.writePartition(appPartition, boot) })
	var refused statusError
	if errors.As(err, &refused) {
		return fmt.Errorf("the module refused to write %s (CBoot said %q) because it doesn't have NVIDIA's partition layout; flash it once with NVIDIA's tools, following the Jetson Nano flashing guide (docs/jetson-nano-flashing.md), then try again", appPartition, refused.msg)
	}
	if err != nil {
		return fmt.Errorf("writing %s: %w", appPartition, err)
	}

	_, _ = fmt.Fprintln(out, "== flushing to storage")
	err = inSession(open, log, func(s *session) error {
		if err := s.command(cmdFlush, nil); err != nil {
			return err
		}
		return s.drain(writeSettle)
	})
	if err != nil {
		return fmt.Errorf("flushing to storage: %w", err)
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

// writePartition sends CBoot's flashing server its {char name[40]; u64 size}
// command (any longer hangs the server until a power cycle) and then the
// partition's data.
func (s *session) writePartition(name string, data *io.SectionReader) error {
	args := make([]byte, nameFieldSize, nameFieldSize+8)
	copy(args, name)
	args = binary.LittleEndian.AppendUint64(args, uint64(data.Size()))
	if err := s.command(cmdWritePartition, args); err != nil {
		return err
	}
	if err := s.drain(quietWait); err != nil {
		return err
	}
	if err := s.stream(data); err != nil {
		return err
	}
	return s.drain(writeSettle)
}
