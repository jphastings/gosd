package jetsonflash

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/l4t"
)

// gosdImage is an MBR whose partition 1 starts at sector 2 and holds two
// sectors of recognisable bytes, with a sector of other content after it.
func gosdImage(t *testing.T) (path string, boot []byte) {
	t.Helper()
	boot = bytes.Repeat([]byte{0xb0}, 2*sectorSize)
	path = filepath.Join(t.TempDir(), "app.img")
	if err := os.WriteFile(path, slices.Concat(mbr(2, 2), make([]byte, sectorSize), boot, make([]byte, sectorSize)), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, boot
}

func mbr(start, sectors uint32) []byte {
	b := make([]byte, sectorSize)
	b[0x1be+4] = 0x0c // FAT32 with LBA, as internal/image writes
	binary.LittleEndian.PutUint32(b[0x1be+8:], start)
	binary.LittleEndian.PutUint32(b[0x1be+12:], sectors)
	b[510], b[511] = 0x55, 0xaa
	return b
}

func openBoot(t *testing.T, path string) *io.SectionReader {
	t.Helper()
	img, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = img.Close() })
	part, err := bootPartition(img)
	if err != nil {
		t.Fatal(err)
	}
	return part
}

func bootChain() l4t.BootChain {
	img := make([]byte, 0x400)
	return l4t.BootChain{RCM0: make([]byte, 1032), RCM1: make([]byte, 1032), CBoot: img, DTB: img}
}

// sessions scripts the boot sequence, then hands out rest in order. It also
// returns the session the BCT is downloaded in.
func sessions(rest ...*fakeDevice) (open func() (conn, error), bct *fakeDevice) {
	bct = &fakeDevice{in: slices.Concat(versionReply(), download(1))}
	devs := slices.Concat([]*fakeDevice{
		{in: [][]byte{make([]byte, 16), le32s(rcmVersion), le32s(0)}},
		bct,
		{in: slices.Concat(versionReply(), download(1), download(3))},
		{in: slices.Concat(versionReply(), [][]byte{devicePacket(packetAck, 1, nil)})},
	}, rest)
	return func() (conn, error) {
		d := devs[0]
		devs = devs[1:]
		return d, nil
	}, bct
}

// storageSession answers the storage query with an eMMC of the given size;
// the bench module's was 16384 boot-device sectors and 30777344 user-area
// sectors.
func storageSession(bootSectors, userSectors uint32) *fakeDevice {
	reply := slices.Concat(make([]byte, 8), le32s(bootSectors, 512), make([]byte, 8), le32s(userSectors, 512))
	return &fakeDevice{in: slices.Concat(versionReply(), [][]byte{
		devicePacket(packetAck, 1, nil), devicePacket(packetCmd, 9, le32s(0x08, 32)),
		devicePacket(packetData, 10, reply), devicePacket(packetCmd, 11, le32s(cmdStatus, 0)), nil,
	})}
}

func sent(d *fakeDevice) []byte { return bytes.Join(d.out, nil) }

func wire(typ, seq uint32, body ...[]byte) []byte {
	return bytes.Join(frame(typ, seq, slices.Concat(body...)), nil)
}

func TestFlashWritesEveryPartitionAndFlushesItBeforeTheBCT(t *testing.T) {
	path, boot := gosdImage(t)
	ff := l4t.FirstFlash{
		GPT:        []byte("gpt"),
		Partitions: []l4t.Partition{{Name: "PT", Data: []byte("pt")}, {Name: "APP"}, {Name: "DTB", Data: []byte("dtb")}},
		BCT:        bctFor(p3448Storage),
	}
	var planned l4t.Storage
	plan := func(s l4t.Storage) (l4t.FirstFlash, error) {
		planned = s
		ff.BCT = bctFor(s)
		return ff, nil
	}
	query := storageSession(16384, 30777344)
	const flushed = ""
	steps := []struct {
		write string // the partition written, or flushed
		data  []byte
		dev   *fakeDevice
	}{{"GPT", ff.GPT, nil}, {"PT", []byte("pt"), nil}, {"APP", boot, nil}, {"DTB", []byte("dtb"), nil}, {flushed, nil, nil}, {"BCT", ff.BCT, nil}, {flushed, nil, nil}}
	devs := []*fakeDevice{query}
	for i, st := range steps {
		steps[i].dev = &fakeDevice{in: slices.Concat(versionReply(), download(1))}
		if st.write == flushed {
			steps[i].dev = &fakeDevice{in: slices.Concat(versionReply(), [][]byte{
				devicePacket(packetAck, 1, nil), devicePacket(packetCmd, 9, le32s(cmdStatus, 0)), nil,
			})}
		}
		devs = append(devs, steps[i].dev)
	}
	reboot := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{devicePacket(packetAck, 1, nil)}), gone: errors.New("ESHUTDOWN")}
	var out bytes.Buffer

	open, rcm := sessions(append(devs, reboot)...)

	if err := flash(open, bootChain(), plan, openBoot(t, path), &out, io.Discard); err != nil {
		t.Fatal(err)
	}

	// CBoot's flashing server finds PT and the rest through the BCT
	// downloaded in recovery mode, so it must be the BCT written last.
	if !bytes.Contains(sent(rcm), wire(packetData, 2, bctFor(p3448Storage))) {
		t.Error("the BCT downloaded in recovery mode isn't the one written last")
	}

	// OEM type 1, sub-command 1, and nothing else: type 9 would provision
	// the eMMC's RPMB key for good.
	oemArgs := unhex(t, "01000000 01000000"+strings.Repeat(" 00000000", 10))
	if !bytes.Contains(sent(query), wire(packetCmd, 1, le32s(cmdOEM), oemArgs)) {
		t.Error("the storage query isn't OEM command 0x05 with exactly type 1, sub-command 1")
	}
	if want := (l4t.Storage{BootSectors: 16384, UserSectors: 30777344}); planned != want {
		t.Errorf("planned for %+v, want %+v", planned, want)
	}
	for i, st := range steps {
		if st.write == flushed {
			if !bytes.Contains(sent(st.dev), wire(packetCmd, 1, le32s(cmdFlush))) {
				t.Errorf("session %d after the query didn't flush", i+1)
			}
			continue
		}
		name := append([]byte(st.write), make([]byte, nameFieldSize-len(st.write))...)
		size := binary.LittleEndian.AppendUint64(nil, uint64(len(st.data)))
		if !bytes.Contains(sent(st.dev), wire(packetCmd, 1, le32s(cmdWritePartition), name, size)) || !bytes.Contains(sent(st.dev), wire(packetData, 2, st.data)) {
			t.Errorf("session %d after the query didn't write %s with its data", i+1, st.write)
		}
	}
	if !bytes.Contains(sent(reboot), wire(packetCmd, 1, le32s(cmdColdBoot, 0))) {
		t.Error("the last session didn't cold-boot")
	}
	if !strings.Contains(out.String(), "the module is booting the new image") {
		t.Errorf("output %q doesn't say the module is booting", out.String())
	}
}

func TestFlashStopsAtAWriteCBootNeverConfirms(t *testing.T) {
	path, _ := gosdImage(t)
	ff := l4t.FirstFlash{GPT: []byte("gpt"), Partitions: []l4t.Partition{{Name: "APP"}}, BCT: []byte("bct")}
	unconfirmed := &fakeDevice{in: slices.Concat(versionReply(), download(1)[:4])}
	opened := 0
	devs, _ := sessions(storageSession(16384, 30777344), unconfirmed)
	open := func() (conn, error) { opened++; return devs() }

	err := flash(open, bootChain(), func(l4t.Storage) (l4t.FirstFlash, error) { return ff, nil }, openBoot(t, path), io.Discard, io.Discard)

	if err == nil || !strings.Contains(err.Error(), "writing GPT") || !strings.Contains(err.Error(), "didn't confirm") {
		t.Fatalf("want an unconfirmed GPT write, got %v", err)
	}
	if !strings.Contains(err.Error(), "recovery mode") || !strings.Contains(err.Error(), "run jetson-flash flash again") {
		t.Errorf("%v doesn't say how to recover", err)
	}
	if opened != 6 {
		t.Errorf("opened the device %d times, want 6 (boot ROM, three boot steps, the query, GPT) and nothing after", opened)
	}
}

// bctFor stands in for a BCT, which depends on the eMMC it's built for.
func bctFor(s l4t.Storage) []byte { return fmt.Appendf(nil, "bct for %+v", s) }

func TestFlashWritesNothingToAnEMMCItsBCTDoesntDescribe(t *testing.T) {
	path, _ := gosdImage(t)
	plan := func(s l4t.Storage) (l4t.FirstFlash, error) {
		return l4t.FirstFlash{GPT: []byte("gpt"), Partitions: []l4t.Partition{{Name: "APP"}}, BCT: bctFor(s)}, nil
	}
	opened := 0
	devs, _ := sessions(storageSession(8192, 30777344))
	open := func() (conn, error) { opened++; return devs() }

	err := flash(open, bootChain(), plan, openBoot(t, path), io.Discard, io.Discard)

	if err == nil || !strings.Contains(err.Error(), "8192-sector boot device") || !strings.Contains(err.Error(), "nothing has been written") {
		t.Fatalf("want a refusal naming the boot device, got %v", err)
	}
	if opened != 5 {
		t.Errorf("opened the device %d times, want 5 (boot ROM, three boot steps, the query) and no write", opened)
	}
}

func TestRunFlashRefusesABadImageBeforeTouchingAnything(t *testing.T) {
	tooBig := uint32(l4t.AppSize/sectorSize + 1)
	linux := mbr(1, 2)
	linux[0x1be+4] = 0x83
	for name, tc := range map[string]struct {
		mbr  []byte
		size int64
		want string
	}{
		"not a disk image": {make([]byte, sectorSize), 4 * sectorSize, "isn't a gosd image"},
		"not FAT32":        {linux, 4 * sectorSize, "isn't a gosd image"},
		"truncated":        {mbr(1, 8), 4 * sectorSize, "truncated"},
		"too big for APP":  {mbr(1, tooBig), int64(1+tooBig) * sectorSize, "too big for the module's 14336 MiB APP partition"},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "app.img")
			if err := os.WriteFile(path, tc.mbr, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(path, tc.size); err != nil {
				t.Fatal(err)
			}

			err := RunFlash(path, FlashOptions{}, io.Discard)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
}

// Each refusal RunFlash can make without the module comes before it is
// opened: no Linux, then no licence (here, no terminal and no flag).
func TestRunFlashRefusesBeforeOpeningTheDevice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path, _ := gosdImage(t)
	want := "--accept-nvidia-license"
	if runtime.GOOS != "linux" {
		want = "needs Linux"
	}

	err := RunFlash(path, FlashOptions{}, io.Discard)

	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v, want an error mentioning %q", err, want)
	}
}
