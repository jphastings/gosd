package jetsonflash

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jphastings/gosd/internal/l4t"
)

const (
	deviceWait = 10 * time.Second
	// bctSettle covers the applet training SDRAM from the BCT before it
	// answers, which took about 0.2 s on the bench.
	bctSettle = time.Second
)

// bootToServer takes a module from the boot ROM to CBoot's flashing server,
// reopening the device for each step as NVIDIA's nvaflash.sh runs a separate
// tool for each. The server finds the boot device's partitions through bct.
// Progress goes to out and the protocol detail to log.
func bootToServer(open func() (conn, error), bc l4t.BootChain, bct []byte, out, log io.Writer) error {
	_, _ = fmt.Fprintln(out, "== talking to the boot ROM")
	err := withConn(open, func(c conn) error {
		uid, err := bootROM(c, bc.RCM0, bc.RCM1)
		if err == nil {
			_, _ = fmt.Fprintf(log, "BR_CID %s: the recovery applet is running\n", brCID(uid))
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("talking to the boot ROM: %w", err)
	}

	steps := []struct {
		name string
		run  func(*session) error
	}{
		{"downloading the BCT", func(s *session) error { return s.download("bct", bct, bctSettle) }},
		{"downloading ebt and rp1", func(s *session) error {
			if err := s.download("ebt", bc.CBoot, quietWait); err != nil {
				return err
			}
			return s.download("rp1", bc.DTB, quietWait)
		}},
		{"booting CBoot's flashing server", (*session).bootFlashingServer},
	}
	for _, st := range steps {
		_, _ = fmt.Fprintf(out, "== %s\n", st.name)
		if err := inSession(open, log, st.run); err != nil {
			return fmt.Errorf("%s: %w", st.name, err)
		}
	}
	return nil
}

// inSession runs f in a fresh session, which opens with the version query.
func inSession(open func() (conn, error), log io.Writer, f func(*session) error) error {
	return withConn(open, func(c conn) error {
		s := &session{c: c, log: log}
		if err := s.version(); err != nil {
			return fmt.Errorf("version query: %w", err)
		}
		return f(s)
	})
}

func withConn(open func() (conn, error), f func(conn) error) error {
	c, err := open()
	if err != nil {
		return err
	}
	return errors.Join(f(c), c.Close())
}

func (s *session) version() error {
	if err := s.command(cmdVersion, nil); err != nil {
		return err
	}
	return s.drain(quietWait)
}

// download sends one image as T210's applet takes it: CMD 0x03 with args
// {char type[4]; u32; u32; u32 length}, which the applet answers with an
// empty CMD 0x08, then one DATA packet carrying the image, then a STATUS
// once it has checked (and, for the BCT, applied) it. The two middle words
// are the load address and entry point from the image's signature header.
func (s *session) download(typ string, img []byte, settle time.Duration) error {
	load, entry := loadAddrs(img)
	args := append([]byte(typ+"\x00"), le32s(load, entry, uint32(len(img)))...)
	if err := s.command(cmdDownload, args); err != nil {
		return err
	}
	if err := s.drain(quietWait); err != nil {
		return err
	}
	if err := s.data(img); err != nil {
		return err
	}
	return s.drain(settle)
}

func (s *session) bootFlashingServer() error {
	if err := s.command(cmdBoot, le32s(bootFlashingServer)); err != nil {
		return err
	}
	return s.drain(quietWait)
}

// loadAddrs reads the load address and entry point from a T210 signature
// header (bytes 0x10 and 0x250), or returns zeros for an image without
// one, or whose header leaves them unspecified.
func loadAddrs(img []byte) (load, entry uint32) {
	const unspecified = 0xfeeddeadbeefbab1
	if len(img) < 0x400 || binary.LittleEndian.Uint64(img[0x10:]) == unspecified {
		return 0, 0
	}
	return le32(img[0x10:]), le32(img[0x250:])
}
