package jetsonflash

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
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
	return l4t.BootChain{RCM0: make([]byte, 1032), RCM1: make([]byte, 1032), BCT: img, CBoot: img, DTB: img}
}

// sessions scripts the boot sequence, then hands out rest in order.
func sessions(rest ...*fakeDevice) func() (conn, error) {
	devs := slices.Concat([]*fakeDevice{
		{in: [][]byte{make([]byte, 16), le32s(rcmVersion), le32s(0)}},
		{in: slices.Concat(versionReply(), download(1))},
		{in: slices.Concat(versionReply(), download(1), download(3))},
		{in: slices.Concat(versionReply(), [][]byte{devicePacket(packetAck, 1, nil)})},
	}, rest)
	return func() (conn, error) {
		d := devs[0]
		devs = devs[1:]
		return d, nil
	}
}

// storageSession answers the storage query as the bench module did: 16384
// boot-device sectors and 30777344 user-area sectors.
func storageSession() *fakeDevice {
	reply := slices.Concat(make([]byte, 8), le32s(16384, 512), make([]byte, 8), le32s(30777344, 512))
	return &fakeDevice{in: slices.Concat(versionReply(), [][]byte{
		devicePacket(packetAck, 1, nil), devicePacket(packetCmd, 9, le32s(0x08, 32)),
		devicePacket(packetData, 10, reply), devicePacket(packetCmd, 11, le32s(cmdStatus, 0)), nil,
	})}
}

func sent(d *fakeDevice) []byte { return bytes.Join(d.out, nil) }

func wire(typ, seq uint32, body ...[]byte) []byte {
	return bytes.Join(frame(typ, seq, slices.Concat(body...)), nil)
}

func TestFlashWritesEveryPartitionWithTheBCTLast(t *testing.T) {
	path, boot := gosdImage(t)
	ff := l4t.FirstFlash{
		GPT:        []byte("gpt"),
		Partitions: []l4t.Partition{{Name: "PT", Data: []byte("pt")}, {Name: "APP"}, {Name: "DTB", Data: []byte("dtb")}},
		BCT:        []byte("bct"),
	}
	var planned l4t.Storage
	plan := func(s l4t.Storage) (l4t.FirstFlash, error) { planned = s; return ff, nil }
	query := storageSession()
	writes := []struct {
		name string
		data []byte
		dev  *fakeDevice
	}{{"GPT", ff.GPT, nil}, {"PT", []byte("pt"), nil}, {"APP", boot, nil}, {"DTB", []byte("dtb"), nil}, {"BCT", ff.BCT, nil}}
	devs := []*fakeDevice{query}
	for i := range writes {
		writes[i].dev = &fakeDevice{in: slices.Concat(versionReply(), download(1))}
		devs = append(devs, writes[i].dev)
	}
	flush := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{
		devicePacket(packetAck, 1, nil), devicePacket(packetCmd, 9, le32s(cmdStatus, 0)), nil,
	})}
	reboot := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{devicePacket(packetAck, 1, nil)}), gone: errors.New("ESHUTDOWN")}
	var out bytes.Buffer

	if err := flash(sessions(append(devs, flush, reboot)...), bootChain(), plan, openBoot(t, path), &out, io.Discard); err != nil {
		t.Fatal(err)
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
	for _, w := range writes {
		name := append([]byte(w.name), make([]byte, nameFieldSize-len(w.name))...)
		size := binary.LittleEndian.AppendUint64(nil, uint64(len(w.data)))
		if !bytes.Contains(sent(w.dev), wire(packetCmd, 1, le32s(cmdWritePartition), name, size)) || !bytes.Contains(sent(w.dev), wire(packetData, 2, w.data)) {
			t.Errorf("the session for %s didn't write it with its data", w.name)
		}
	}
	if !bytes.Contains(sent(flush), wire(packetCmd, 1, le32s(cmdFlush))) {
		t.Error("the session after the BCT didn't flush")
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
	devs := sessions(storageSession(), unconfirmed)
	open := func() (conn, error) { opened++; return devs() }

	err := flash(open, bootChain(), func(l4t.Storage) (l4t.FirstFlash, error) { return ff, nil }, openBoot(t, path), io.Discard, io.Discard)

	if err == nil || !strings.Contains(err.Error(), "writing GPT") || !strings.Contains(err.Error(), "didn't confirm") {
		t.Fatalf("want an unconfirmed GPT write, got %v", err)
	}
	if opened != 6 {
		t.Errorf("opened the device %d times, want 6 (boot ROM, three boot steps, the query, GPT) and nothing after", opened)
	}
}

func TestRunFlashRefusesABadImageBeforeTouchingAnything(t *testing.T) {
	tooBig := uint32(l4t.AppSize/sectorSize + 1)
	for name, tc := range map[string]struct {
		mbr  []byte
		size int64
		want string
	}{
		"not a disk image": {make([]byte, sectorSize), 4 * sectorSize, "isn't a gosd image"},
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
