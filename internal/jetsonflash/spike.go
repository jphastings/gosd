// Package jetsonflash drives a Jetson Nano (Tegra210) eMMC module in USB
// recovery mode without NVIDIA's host tools (bean gosd-32dw). For now it is
// a spike: it boots the module as far as CBoot's flashing server, sending
// files NVIDIA's own tools already signed.
package jetsonflash

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// SpikeOptions are the T210 nv3p facts the spike is still finding out on
// the bench.
type SpikeOptions struct {
	// VersionCmd is the command every session opens with: NVIDIA's tools
	// print "Applet version" (or "Cboot version") at the start of each run.
	// Zero means tegrarcm's GET_PLATFORM_INFO.
	VersionCmd uint32
	// BootCmd boots the applet into CBoot's flashing server. Zero stops
	// after downloading rp1.
	BootCmd uint32
	// BootArgs are BootCmd's arguments.
	BootArgs []byte
}

const (
	deviceWait = 10 * time.Second
	// bctSettle covers the applet training SDRAM from the BCT before it
	// answers, which took about 0.2 s on the bench.
	bctSettle = time.Second
)

type spikeFiles struct{ rcm0, rcm1, bct, ebt, rp1 []byte }

// RunSpike boots a module waiting in USB recovery mode as far as CBoot's
// flashing server, using the NVIDIA-signed files in dir (an unpacked
// massflash blob), and logs every USB transfer to log. It writes nothing to
// the module's storage.
func RunSpike(dir string, o SpikeOptions, log io.Writer) error {
	f, err := loadSpikeFiles(dir)
	if err != nil {
		return err
	}
	start := time.Now()
	return spike(func() (conn, error) {
		c, err := openRecovery(deviceWait)
		if err != nil {
			return nil, err
		}
		return tracedConn{conn: c, w: log, start: start}, nil
	}, f, o, log)
}

func loadSpikeFiles(dir string) (spikeFiles, error) {
	var f spikeFiles
	for _, x := range []struct {
		name string
		dst  *[]byte
	}{
		{"rcm_0_encrypt.rcm", &f.rcm0},
		{"rcm_1_encrypt.rcm", &f.rcm1},
		{"P3448_A00_lpddr4_204Mhz_P987.bct", &f.bct},
		{"cboot.bin", &f.ebt},
		{"kernel_tegra210-p3448-0002-p3449-0000-b00.dtb", &f.rp1},
	} {
		b, err := os.ReadFile(filepath.Join(dir, x.name))
		if err != nil {
			return f, fmt.Errorf("reading NVIDIA's signed files: %w", err)
		}
		if filepath.Ext(x.name) == ".rcm" {
			if err := checkRCM(b); err != nil {
				return f, fmt.Errorf("%s isn't a signed RCM message: %w", x.name, err)
			}
		}
		*x.dst = b
	}
	return f, nil
}

// spike reopens the device for each step, as NVIDIA's nvaflash.sh runs a
// separate tool for each, and each of those opens with the version query.
func spike(open func() (conn, error), f spikeFiles, o SpikeOptions, log io.Writer) error {
	err := withConn(open, func(c conn) error {
		uid, err := bootROM(c, f.rcm0, f.rcm1)
		if err == nil {
			_, _ = fmt.Fprintf(log, "BR_CID %s: the recovery applet is running\n", brCID(uid))
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("talking to the boot ROM: %w", err)
	}

	type step struct {
		name string
		run  func(*session) error
	}
	steps := []step{
		{"downloading the BCT", func(s *session) error { return s.download("bct", f.bct, bctSettle) }},
		{"downloading ebt and rp1", func(s *session) error {
			if err := s.download("ebt", f.ebt, quietWait); err != nil {
				return err
			}
			return s.download("rp1", f.rp1, quietWait)
		}},
	}
	if o.BootCmd != 0 {
		steps = append(steps,
			step{"booting recovery", func(s *session) error { return s.boot(o.BootCmd, o.BootArgs) }},
			step{"querying CBoot's flashing server", func(*session) error { return nil }},
		)
	}
	for _, st := range steps {
		_, _ = fmt.Fprintf(log, "== %s\n", st.name)
		err := withConn(open, func(c conn) error {
			s := &session{c: c, log: log}
			if err := s.version(cmp.Or(o.VersionCmd, cmdGetPlatformInfo)); err != nil {
				return fmt.Errorf("version query: %w", err)
			}
			return st.run(s)
		})
		if err != nil {
			return fmt.Errorf("%s: %w", st.name, err)
		}
	}
	if o.BootCmd == 0 {
		_, _ = fmt.Fprintln(log, "stopped after rp1: no boot command given")
	}
	return nil
}

func withConn(open func() (conn, error), f func(conn) error) error {
	c, err := open()
	if err != nil {
		return err
	}
	return errors.Join(f(c), c.Close())
}

func (s *session) version(cmd uint32) error {
	if err := s.command(cmd, nil); err != nil {
		return err
	}
	return s.drain(quietWait)
}

// download sends one image as T210's applet takes it: CMD 0x03 with args
// {char type[4]; u32; u32; u32 length}, which the applet answers with an
// empty CMD 0x08, then one DATA packet carrying the image, then a STATUS
// once it has checked (and, for the BCT, applied) it. The two middle words
// are zero, as nvaflash.sh passes "0 0" for the load address and entry.
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

func (s *session) boot(cmd uint32, args []byte) error {
	if err := s.command(cmd, args); err != nil {
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
