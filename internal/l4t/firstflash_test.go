package l4t

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// syntheticLayout is a small layout shaped like NVIDIA's. Each row is
// "name type size attribute [file [start]]".
func syntheticLayout() []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><partition_layout version="01.00.0000">`)
	for _, dev := range [][]string{{
		"BCT boot_config_table 8192 8",
		"NVC bootloader 8192 8 nvtboot.bin",
		"PT partition_table 8192 8 flash.xml.bin",
		"EBT bootloader 8192 8 EBTFILE",
		"NVC-1 bootloader 8192 8 nvtboot.bin",
		"PT-1 partition_table 8192 8 flash.xml.bin",
		"EBT-1 bootloader 8192 8 EBTFILE",
		"VER data 8192 8 VERFILE 0x20000",
	}, {
		"GP1 GP1 2097152 8",
		"APP data APPSIZE 8 APPFILE",
		"DTB data 8192 8 DTBFILE", "TOS data 8192 8 TOSFILE", "EKS data 8192 8 EKSFILE", "LNX data 8192 8 LNXFILE",
		"DTB-1 data 8192 8 DTBFILE", "TOS-1 data 8192 8 TOSFILE", "EKS-1 data 8192 8 EKSFILE", "LNX-1 data 8192 8 LNXFILE",
		"WB0 WB0 8192 8 WB0FILE", "BPF data 8192 8 BPFFILE", "TBC bootloader 8192 8 TBCFILE",
		"UDA data 1048576 0x808",
		"GPT GPT -1 8",
	}} {
		b.WriteString(`<device type="sdmmc" instance="3">`)
		for _, row := range dev {
			f := append(strings.Fields(row), "", "")
			fmt.Fprintf(&b, `<partition name="%s" type="%s"><size> %s </size><allocation_attribute> %s </allocation_attribute>`, f[0], f[1], f[2], f[3])
			if f[4] != "" {
				fmt.Fprintf(&b, `<filename> %s </filename>`, f[4])
			}
			if f[5] != "" {
				fmt.Fprintf(&b, `<start_location> %s </start_location>`, f[5])
			}
			b.WriteString(`</partition>`)
		}
		b.WriteString(`</device>`)
	}
	b.WriteString(`</partition_layout>`)
	return []byte(b.String())
}

// syntheticStorage fits the synthetic layout's boot device exactly.
var syntheticStorage = Storage{BootSectors: 0x20000/512 + 16, UserSectors: 30777344}

func firstFlash(t *testing.T, in Inputs, s Storage) (FirstFlash, map[string][]byte) {
	t.Helper()
	return firstFlashFrom(t, in, s, rand.Reader)
}

func firstFlashFrom(t *testing.T, in Inputs, s Storage, random io.Reader) (FirstFlash, map[string][]byte) {
	t.Helper()
	ff, err := BuildFirstFlash(in, s, time.Now(), random)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, p := range ff.Partitions {
		files[p.Name] = p.Data
	}
	return ff, files
}

func TestFirstFlashWritesInNVIDIAsOrder(t *testing.T) {
	ff, _ := firstFlash(t, syntheticInputs(), syntheticStorage)
	var names []string
	for _, p := range ff.Partitions {
		names = append(names, p.Name)
		if (p.Data == nil) != (p.Name == "APP") {
			t.Errorf("%s has %d bytes; only APP should be left to the caller", p.Name, len(p.Data))
		}
	}
	want := "PT PT-1 NVC EBT NVC-1 EBT-1 VER APP DTB TOS EKS LNX DTB-1 TOS-1 EKS-1 LNX-1 WB0 BPF TBC"
	if got := strings.Join(names, " "); got != want {
		t.Errorf("write order\n got %s\nwant %s", got, want)
	}
}

// counter is a random source whose bytes count up, so every draw is known.
type counter byte

func (c *counter) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(*c)
		*c++
	}
	return len(p), nil
}

func drawn(n int) [16]byte {
	var g [16]byte
	for i := range g {
		g[i] = byte(16*n + i)
	}
	g[7] = g[7]&0x0F | 0x40
	g[8] = g[8]&0x3F | 0x80
	return g
}

type gptEntry struct {
	name        string
	guid        [16]byte
	first, last uint64
}

func readGPT(gpt []byte) []gptEntry {
	var out []gptEntry
	for e := gpt[512:]; e[0] != 0; e = e[128:] {
		name := make([]uint16, 36)
		for i := range name {
			name[i] = binary.LittleEndian.Uint16(e[56+2*i:])
		}
		out = append(out, gptEntry{
			name:  strings.TrimRight(string(utf16.Decode(name)), "\x00"),
			guid:  [16]byte(e[16:32]),
			first: binary.LittleEndian.Uint64(e[32:]),
			last:  binary.LittleEndian.Uint64(e[40:]),
		})
	}
	return out
}

func TestEachGPTEntryCarriesItsPartitionTableGUID(t *testing.T) {
	var c counter
	ff, files := firstFlashFrom(t, syntheticInputs(), syntheticStorage, &c)
	pt := files["PT"]
	le := binary.LittleEndian
	if le.Uint32(pt[len(pt)-4:]) != crc32.ChecksumIEEE(pt[:len(pt)-4]) || string(pt[len(pt)-8:len(pt)-4]) != "PTHD" {
		t.Error("the partition table's trailer is not a PTHD CRC over the rest")
	}

	entries, _ := walkPartitionTable(pt)
	guids := map[string][16]byte{}
	for n, e := range entries {
		nameAt, fileAt := le.Uint32(pt[e:]), le.Uint32(pt[e+0x54:])
		name, _, _ := strings.Cut(string(pt[nameAt:]), "\x00")
		if fileAt != 0 && fileAt != nameAt+uint32(len(name))+1 {
			t.Errorf("%s's filename pointer %#x is not the offset of its filename", name, fileAt)
		}
		guids[name] = [16]byte(pt[e+0x3C:])
		if guids[name] != drawn(n) {
			t.Errorf("%s's GUID is %x, want draw %d stamped as version 4", name, guids[name], n)
		}
	}
	for _, e := range readGPT(ff.GPT) {
		if e.guid != guids[e.name] {
			t.Errorf("GPT entry %s has GUID %x, its partition table entry %x", e.name, e.guid, guids[e.name])
		}
	}
	if [16]byte(ff.GPT[56:72]) != drawn(len(entries)) {
		t.Error("the disk GUID is not drawn after every partition's")
	}
}

func TestGPTSpansTheUserArea(t *testing.T) {
	ff, _ := firstFlash(t, syntheticInputs(), syntheticStorage)
	h, n := bytes.Clone(ff.GPT[:92]), uint64(syntheticStorage.UserSectors)
	le := binary.LittleEndian
	if le.Uint32(h[88:]) != crc32.ChecksumIEEE(ff.GPT[512:]) {
		t.Error("the entries CRC is wrong")
	}
	headerCRC := le.Uint32(h[16:])
	le.PutUint32(h[16:], 0)
	if headerCRC != crc32.ChecksumIEEE(h) {
		t.Error("the header CRC is wrong")
	}
	for _, f := range []struct {
		name string
		at   int
		want uint64
	}{{"MyLBA", 24, 1}, {"AlternateLBA", 32, n - 1}, {"FirstUsableLBA", 40, 34}, {"LastUsableLBA", 48, n - 33}, {"PartitionEntryLBA", 72, 2}} {
		if got := le.Uint64(h[f.at:]); got != f.want {
			t.Errorf("%s = %d, want %d", f.name, got, f.want)
		}
	}

	entries := readGPT(ff.GPT)
	var names []string
	next := uint64(34)
	for _, e := range entries {
		names = append(names, e.name)
		if e.first != next {
			t.Errorf("%s starts at LBA %d, want %d", e.name, e.first, next)
		}
		next = e.last + 1
	}
	if app := entries[0]; app.last-app.first+1 != 14<<30/512 {
		t.Errorf("APP is %d sectors, want 14 GiB", app.last-app.first+1)
	}
	if uda := entries[len(entries)-1]; uda.last != n-34 {
		t.Errorf("UDA ends at LBA %d, want %d, just before the secondary GPT", uda.last, n-34)
	}
	if got := strings.Join(names, " "); got != "APP DTB TOS EKS LNX DTB-1 TOS-1 EKS-1 LNX-1 WB0 BPF TBC UDA" {
		t.Errorf("GPT entries are %s, want the user area's partitions without GP1 or GPT", got)
	}
}

func TestFirstFlashRefusesAnEMMCTooSmallForTheLayout(t *testing.T) {
	ff, _ := firstFlash(t, syntheticInputs(), syntheticStorage)
	entries := readGPT(ff.GPT)
	// UDA must keep its listed 1 MiB, and the secondary GPT its 33 sectors.
	smallest := Storage{BootSectors: syntheticStorage.BootSectors, UserSectors: int64(entries[len(entries)-1].first) + 2048 + 33}
	if _, err := BuildFirstFlash(syntheticInputs(), smallest, time.Now(), rand.Reader); err != nil {
		t.Errorf("the smallest user area that fits was refused: %v", err)
	}
	for name, s := range map[string]Storage{
		"user area":   {BootSectors: smallest.BootSectors, UserSectors: smallest.UserSectors - 1},
		"boot device": {BootSectors: smallest.BootSectors - 1, UserSectors: smallest.UserSectors},
	} {
		if _, err := BuildFirstFlash(syntheticInputs(), s, time.Now(), rand.Reader); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("a %s one sector short: want an error naming it, got %v", name, err)
		}
	}
}

func TestFinalBCTDescribesWhatEachCopyBoots(t *testing.T) {
	in := syntheticInputs()
	ff, files := firstFlash(t, in, syntheticStorage)
	b := ff.BCT
	u32 := func(at int) uint32 { return binary.LittleEndian.Uint32(b[at:]) }
	assertSigned(t, "bct", b, 0x310, 0x510, 0x2800)

	if u32(0x232C) != 2 {
		t.Errorf("BCT lists %d bootloaders, want NVC and NVC-1", u32(0x232C))
	}
	nvtbootCMAC := cmac(zeroKey, in.NVTBoot)
	for i, want := range [][7]uint32{
		{1, 0, 16, 1000, 0x40010000, 0x40010000, 2}, // NVC at 8 KiB, the layout's second partition
		{1, 2, 0, 1000, 0x40010000, 0x40010000, 5},  // NVC-1 at 32 KiB
	} {
		e := 0x2330 + i*0x12C
		for f, w := range want {
			if got := u32(e + 4*f); got != w {
				t.Errorf("bootloader %d word %d = %#x, want %#x", i, f, got, w)
			}
		}
		if !bytes.Equal(b[e+0x1C:e+0x2C], nvtbootCMAC[:]) {
			t.Errorf("bootloader %d does not carry nvtboot.bin's CMAC", i)
		}
	}

	// A set covers its files each zero-padded to its 8 KiB partition, but a
	// kernel set stops at the end of its last file.
	set := func(kernel bool, names ...string) []byte {
		var r []byte
		for i, n := range names {
			r = append(r, files[n]...)
			if !kernel || i < len(names)-1 {
				r = append(r, make([]byte, 8192-len(files[n]))...)
			}
		}
		return r
	}
	dtb := uint32(syntheticStorage.BootSectors) + 34 + 14<<30/512
	for _, c := range []struct {
		name   string
		at     int
		files  []byte
		sector uint32
	}{
		{"BFS0", 0x4C0, set(false, "PT", "EBT"), 32},
		{"BFS1", 0x4C4, set(false, "PT-1", "EBT-1", "VER"), 80},
		{"KFS0", 0x480, set(true, "DTB", "TOS", "EKS", "LNX"), dtb},
		{"KFS1", 0x484, set(true, "DTB-1", "TOS-1", "EKS-1", "LNX-1"), dtb + 64},
	} {
		if u32(c.at) != uint32(len(c.files)) || u32(c.at+0x10) != xor32(c.files) || u32(c.at+0x20) != c.sector {
			t.Errorf("%s is %d bytes, SUM %#x at sector %d; want %d bytes, SUM %#x at sector %d",
				c.name, u32(c.at), u32(c.at+0x10), u32(c.at+0x20), len(c.files), xor32(c.files), c.sector)
		}
	}
	if u32(0x4F0) != 8192 || u32(0x4F4) != 8192 {
		t.Error("BFS entries do not record their partition table's size")
	}
}

func TestXOR32PadsAPartialWord(t *testing.T) {
	if got := xor32([]byte{1, 0, 0, 0, 1, 0, 0, 0}); got != 0 {
		t.Errorf("identical words XOR to %#x, want 0", got)
	}
	if got := xor32([]byte{1, 2, 3, 4, 5}); got != 0x04030201^5 {
		t.Errorf("xor32 = %#x, want the partial word zero-padded", got)
	}
}

func TestFirstFlashSignsItsBootFiles(t *testing.T) {
	_, files := firstFlash(t, syntheticInputs(), syntheticStorage)
	for _, c := range []struct {
		name             string
		cmacAt, signedAt int
	}{{"TBC", 0x118, 0x230}, {"WB0", 0x110, 0x220}, {"BPF", 0x118, 0x230}, {"LNX", 0x6B8, 0x7D0}} {
		assertSigned(t, c.name, files[c.name], c.cmacAt, c.signedAt, len(files[c.name]))
	}
	tos := files["TOS"]
	assertSigned(t, "TOS", tos, len(tos)-16, 0, len(tos)-16)

	if lnx := files["LNX"]; string(lnx[:8]) != "ANDROID!" || string(lnx[2048:2054]) != "u-boot" || len(lnx) != 4096 {
		t.Error("LNX is not a page-aligned Android boot image of u-boot.bin")
	}
	if bpf := files["BPF"]; string(bpf[4:8]) != "SC7F" || string(bpf[0x400:0x403]) != "sc7" {
		t.Error("BPF is not sc7entry-firmware.bin behind an SC7F header")
	}
}

func TestFirstFlashRefusesDamagedKitFiles(t *testing.T) {
	for name, damage := range map[string]func(*Inputs){
		"TOS in partial blocks":         func(in *Inputs) { in.TOS = in.TOS[:31] },
		"warmboot with a header":        func(in *Inputs) { in.WarmBoot[0] = 1 },
		"nvtboot_cpu without a header":  func(in *Inputs) { in.NVTBootCPU = in.NVTBootCPU[:0x3FF] },
		"EKS bigger than its partition": func(in *Inputs) { in.EKS = make([]byte, 8193) },
		"unknown partition file":        func(in *Inputs) { in.Layout = bytes.Replace(in.Layout, []byte("EKSFILE"), []byte("surprise.bin"), 1) },
	} {
		in := syntheticInputs()
		damage(&in)
		if _, err := BuildFirstFlash(in, syntheticStorage, time.Now(), rand.Reader); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestVersionFileCarriesItsCksum(t *testing.T) {
	got := string(versionFile(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)))
	want := "NV3\n# R32 , REVISION: 7.6\nBOARDID=3448 BOARDSKU=0002 FAB=401\n20300102030405\nBYTES:76 CRC32:2608751674\n"
	if got != want {
		t.Errorf("version file\n got %q\nwant %q", got, want)
	}
}

func TestCksumMatchesPOSIX(t *testing.T) {
	// From the cksum(1) utility.
	for in, want := range map[string]uint32{
		"":                        4294967295,
		"abc\n":                   1112837078,
		"123456789":               930766865,
		string(make([]byte, 300)): 351385237,
	} {
		if got := cksum([]byte(in)); got != want {
			t.Errorf("cksum of %d bytes = %d, want %d", len(in), got, want)
		}
	}
}

// walkPartitionTable follows a gosd-built partition table's pointer words,
// which hold real offsets, to its entries; it returns the entries' offsets
// and the pointer words'.
func walkPartitionTable(pt []byte) (entries, pointers []int) {
	le := binary.LittleEndian
	pointers = []int{8}
	for d := 0; d < int(le.Uint32(pt[4:])); d++ {
		rec := 12 + 16*d
		pointers = append(pointers, rec+12)
		at := int(le.Uint32(pt[rec+12:]))
		for i := 0; i < int(le.Uint32(pt[rec+8:])); i++ {
			e := at + ptEntryLen*i
			entries = append(entries, e)
			pointers = append(pointers, e, e+0x54)
		}
	}
	return entries, pointers
}
