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
	"unicode/utf16"

	"github.com/jphastings/gosd/internal/l4t"
)

// fatFS and ext4FS are four sectors that carry just the signatures
// bootPartition looks for, then recognisable filler.
func fatFS() []byte  { return fsWith(0x52, []byte("FAT32   ")) }
func ext4FS() []byte { return fsWith(1080, []byte{0x53, 0xef}) }

func fsWith(at int, magic []byte) []byte {
	b := bytes.Repeat([]byte{0xb0}, 4*sectorSize)
	copy(b, make([]byte, 1024))
	copy(b[at:], magic)
	return b
}

// mbrImage is an MBR whose partition 1 starts at sector 2, with a sector of
// other content after it.
func mbrImage(typ byte, fs []byte) []byte {
	b := make([]byte, sectorSize)
	b[0x1be+4] = typ
	binary.LittleEndian.PutUint32(b[0x1be+8:], 2)
	binary.LittleEndian.PutUint32(b[0x1be+12:], uint32(len(fs)/sectorSize))
	b[510], b[511] = 0x55, 0xaa
	return slices.Concat(b, make([]byte, sectorSize), fs, make([]byte, sectorSize))
}

type gptPart struct {
	name string
	fs   []byte
}

// gptImage has its header at sector 1, four entries at sector 2, and the
// partitions from sector 3.
func gptImage(parts ...gptPart) []byte {
	hdr := make([]byte, sectorSize)
	copy(hdr, "EFI PART")
	binary.LittleEndian.PutUint64(hdr[72:], 2)
	binary.LittleEndian.PutUint32(hdr[80:], 4)
	binary.LittleEndian.PutUint32(hdr[84:], 128)
	entries := make([]byte, 4*128)
	var data []byte
	next := uint64(3)
	for i, p := range parts {
		e := entries[i*128:]
		e[0] = 1
		sectors := uint64(len(p.fs) / sectorSize)
		binary.LittleEndian.PutUint64(e[32:], next)
		binary.LittleEndian.PutUint64(e[40:], next+sectors-1)
		for j, u := range utf16.Encode([]rune(p.name)) {
			binary.LittleEndian.PutUint16(e[56+2*j:], u)
		}
		next += sectors
		data = append(data, p.fs...)
	}
	return slices.Concat(make([]byte, sectorSize), hdr, entries, data)
}

func writeImage(t *testing.T, b []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.img")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// gosdImage is an image in gosd's shape: an MBR with a FAT32 partition 1.
func gosdImage(t *testing.T) (path string, boot []byte) {
	t.Helper()
	boot = fatFS()
	return writeImage(t, mbrImage(0x0c, boot)), boot
}

func openBoot(t *testing.T, path string) *io.SectionReader {
	t.Helper()
	part, err := bootPartitionOf(path)
	if err != nil {
		t.Fatal(err)
	}
	return part
}

func bootPartitionOf(path string) (*io.SectionReader, error) {
	img, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := img.Stat()
	if err != nil {
		return nil, err
	}
	return bootPartition(img, info.Size())
}

func TestBootPartitionPicksAPPFromEveryImageShape(t *testing.T) {
	for name, tc := range map[string]struct {
		image []byte
		want  []byte // APP's content, or nil if the image must be refused
		err   string
	}{
		"MBR with FAT32 (gosd)":  {mbrImage(0x0c, fatFS()), fatFS(), ""},
		"MBR with ext4":          {mbrImage(0x83, ext4FS()), ext4FS(), ""},
		"GPT with APP second":    {gptImage(gptPart{"esp", fatFS()}, gptPart{"APP", ext4FS()}), ext4FS(), ""},
		"GPT without APP":        {gptImage(gptPart{"rootfs", fatFS()}, gptPart{"data", ext4FS()}), fatFS(), ""},
		"bare ext4":              {ext4FS(), ext4FS(), ""},
		"bare FAT":               {fatFS(), fatFS(), ""},
		"no layout":              {make([]byte, 4*sectorSize), nil, "no GPT, no MBR and no ext4"},
		"MBR with other content": {mbrImage(0x83, fsWith(0, nil)), nil, "neither FAT nor ext4"},
		"GPT with other content": {gptImage(gptPart{"APP", fsWith(0, nil)}), nil, "neither FAT nor ext4"},
		"MBR cut short":          {mbrImage(0x83, ext4FS())[:5*sectorSize], nil, "truncated"},
		"GPT with no partitions": {gptImage(), nil, "no partitions"},
	} {
		t.Run(name, func(t *testing.T) {
			part, err := bootPartitionOf(writeImage(t, tc.image))

			if tc.want == nil {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("got %v, want an error mentioning %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := io.ReadAll(part); !bytes.Equal(got, tc.want) {
				t.Errorf("APP isn't the expected filesystem (%d bytes, want %d)", len(got), len(tc.want))
			}
		})
	}
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

func TestRunFlashRefusesAnImageTooBigForAPPBeforeTouchingAnything(t *testing.T) {
	sectors := uint32(l4t.AppSize/sectorSize + 1)
	b := mbrImage(0x0c, fatFS())
	binary.LittleEndian.PutUint32(b[0x1be+12:], sectors)
	path := writeImage(t, b)
	if err := os.Truncate(path, int64(2+sectors)*sectorSize); err != nil {
		t.Fatal(err)
	}

	err := RunFlash(path, FlashOptions{}, io.Discard)

	if err == nil || !strings.Contains(err.Error(), "too big for the module's 14336 MiB APP partition") {
		t.Fatalf("got %v, want a too-big refusal", err)
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
