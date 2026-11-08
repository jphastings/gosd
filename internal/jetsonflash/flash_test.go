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
)

// gosdImage is an MBR whose partition 1 starts at sector 2 and holds two
// sectors of recognisable bytes, with a sector of other content after it.
func gosdImage(t *testing.T) (path string, boot []byte) {
	t.Helper()
	mbr := make([]byte, sectorSize)
	binary.LittleEndian.PutUint32(mbr[0x1be+8:], 2)
	binary.LittleEndian.PutUint32(mbr[0x1be+12:], 2)
	mbr[510], mbr[511] = 0x55, 0xaa
	boot = bytes.Repeat([]byte{0xb0}, 2*sectorSize)
	path = filepath.Join(t.TempDir(), "app.img")
	if err := os.WriteFile(path, slices.Concat(mbr, make([]byte, sectorSize), boot, make([]byte, sectorSize)), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, boot
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

func flashFiles() spikeFiles {
	img := make([]byte, 0x400)
	return spikeFiles{rcm0: make([]byte, 1032), rcm1: make([]byte, 1032), bct: img, ebt: img, rp1: img}
}

// serverDevices scripts the boot sequence, then the write, flush and cold
// boot sessions as given.
func serverDevices(rest ...*fakeDevice) func() (conn, error) {
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

func sent(d *fakeDevice) []byte { return bytes.Join(d.out, nil) }

func TestFlashRewritesAPPThenFlushesAndColdBoots(t *testing.T) {
	path, boot := gosdImage(t)
	part := openBoot(t, path)
	write := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{
		devicePacket(packetAck, 1, nil), devicePacket(packetCmd, 9, le32s(0x08, 0)), nil,
		devicePacket(packetAck, 2, nil), devicePacket(packetCmd, 9, le32s(cmdStatus, 0)), nil,
	})}
	flush := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{
		devicePacket(packetAck, 1, nil), devicePacket(packetCmd, 9, le32s(cmdStatus, 0)), nil,
	})}
	reboot := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{devicePacket(packetAck, 1, nil)}), gone: errors.New("ESHUTDOWN")}
	var out bytes.Buffer

	if err := flash(serverDevices(write, flush, reboot), flashFiles(), part, &out, io.Discard); err != nil {
		t.Fatal(err)
	}

	args := append([]byte("APP"), make([]byte, 37)...)
	args = binary.LittleEndian.AppendUint64(args, uint64(len(boot)))
	for _, c := range []struct {
		name string
		dev  *fakeDevice
		want [][]byte
	}{
		{"write", write, frame(packetCmd, 1, append(le32s(cmdWritePartition), args...))},
		{"data", write, frame(packetData, 2, boot)},
		{"flush", flush, frame(packetCmd, 1, le32s(cmdFlush))},
		{"cold boot", reboot, frame(packetCmd, 1, le32s(cmdColdBoot, 0))},
	} {
		if !bytes.Contains(sent(c.dev), bytes.Join(c.want, nil)) {
			t.Errorf("the %s session never sent the expected packet", c.name)
		}
	}
	if !strings.Contains(out.String(), "the module is booting the new image") {
		t.Errorf("output %q doesn't say the module is booting", out.String())
	}
}

func TestFlashExplainsAMissingPartitionLayoutAndDoesNotReboot(t *testing.T) {
	path, _ := gosdImage(t)
	part := openBoot(t, path)
	write := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{
		devicePacket(packetAck, 1, nil), devicePacket(packetCmd, 9, append(le32s(cmdStatus, 4), "Partition not found"...)), nil,
	})}
	opened := 0
	devs := serverDevices(write)
	open := func() (conn, error) { opened++; return devs() }

	err := flash(open, flashFiles(), part, io.Discard, io.Discard)

	if err == nil || !strings.Contains(err.Error(), "Partition not found") || !strings.Contains(err.Error(), "NVIDIA's partition layout") {
		t.Fatalf("want the actionable refusal, got %v", err)
	}
	if opened != 5 {
		t.Errorf("opened the device %d times, want 5 (boot ROM, three boot steps, the write) and no cold boot", opened)
	}
}

func TestRunFlashRejectsANonGosdImageBeforeOpeningTheDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blank.img")
	if err := os.WriteFile(path, make([]byte, 4*sectorSize), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RunFlash(path, t.TempDir(), io.Discard, nil)

	if err == nil || !strings.Contains(err.Error(), "isn't a gosd image") || !strings.Contains(err.Error(), "gosd build --board jetson-nano") {
		t.Fatalf("want a not-a-gosd-image error, got %v", err)
	}
}
