package l4t

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// Storage is the eMMC's size, in 512-byte sectors, as CBoot's storage query
// reports it.
type Storage struct {
	BootSectors, UserSectors int64
}

// FirstFlash is every file a first flash writes, for a module that has never
// had NVIDIA's layout on it. CBoot's flashing server takes GPT first, since
// writing it erases the eMMC and lays out the user area, then Partitions in
// order, then the BCT. The BCT must also be the one downloaded in recovery
// mode, before the eMMC's size can be read; it depends on the boot device's
// size but not the user area's.
type FirstFlash struct {
	GPT        []byte
	Partitions []Partition
	BCT        []byte
}

// Partition is one named partition write. APP's Data is nil: the caller
// writes the app's boot volume there.
type Partition struct {
	Name string
	Data []byte
}

// BuildFirstFlash builds a first flash from the kit, laid out for the eMMC s
// describes. The version file records now, and every partition GUID is drawn
// from random (crypto/rand's Reader in production).
func BuildFirstFlash(in Inputs, s Storage, now time.Time, random io.Reader) (FirstFlash, error) {
	bc, err := Build(in)
	if err != nil {
		return FirstFlash{}, err
	}
	l, err := newLayout(in.Layout, s, random)
	if err != nil {
		return FirstFlash{}, err
	}
	files, err := partitionFiles(in, bc, now)
	if err != nil {
		return FirstFlash{}, err
	}
	if files[ptFile], err = l.partitionTable(); err != nil {
		return FirstFlash{}, err
	}
	if err := l.attach(files); err != nil {
		return FirstFlash{}, err
	}
	bct, err := l.finalBCT(bc.bct)
	if err != nil {
		return FirstFlash{}, err
	}
	return FirstFlash{GPT: l.gpt(), Partitions: l.writes(), BCT: bct}, nil
}

const (
	// AppSize is APP's size in bytes: NVIDIA's default ROOTFSSIZE. The
	// flashed OS's filesystem, which U-Boot boots from, goes in APP.
	AppSize = 14 << 30

	dtbFile = "kernel_tegra210-p3448-0002-p3449-0000-b00.dtb"
	ptFile  = "crc-flash.xml.bin"
	appFile = "system.img"
)

// layoutTags are flash.sh's substitutions into the layout template for this
// module (p3449-0000+p3448-0002, FAB 300 or later).
var layoutTags = strings.NewReplacer(
	"APPSIZE", strconv.FormatInt(AppSize, 10),
	"NXC", "NVC", "NVCTYPE", "bootloader", "NVCFILE", "nvtboot.bin",
	"TXC", "TBC", "TBCTYPE", "bootloader", "TBCFILE", "nvtboot_cpu.bin",
	"WX0", "WB0", "WB0TYPE", "WB0", "WB0FILE", "warmboot.bin",
	"BXF", "BPF", "BPFFILE", "sc7entry-firmware.bin",
	// flash.sh deletes these filename lines, leaving BPF-DTB empty.
	"BPFDTB-FILE", "",
	"DXB", "DTB", "DTBFILE", dtbFile, "EBTFILE", "cboot.bin",
	"TXS", "TOS", "TOSFILE", "tos-mon-only.img", "EXS", "EKS", "EKSFILE", "eks.img",
	"LNXFILE", "boot.img", "APPFILE", appFile, "VERFILE", "emmc_bootblob_ver.txt",
	"flash.xml.bin", ptFile,
	"RECNAME", "recovery", "RECDTB-NAME", "recovery-dtb", "BOOTCTRLNAME", "kernel-bootctrl",
	"RECROOTFSSIZE", "314572800",
)

var partitionTypes = map[string]uint32{
	"boot_config_table": 1, "bootloader": 2, "partition_table": typePartitionTable,
	"data": 5, "GP1": typeGP1, "GPT": typeGPT, "WB0": 14,
}

const (
	typePartitionTable = 3
	typeGP1            = 8
	typeGPT            = 9

	// attrRest marks the partition that takes the rest of the device.
	attrRest = 0x800

	sector      = 512
	gptFirstLBA = 34
	gptEntries  = 128
	gptEntryLen = 128
)

type partition struct {
	name, file    string
	id, typ, attr uint32
	// start is the layout's start_location, size is -1 for "the rest", and
	// first and sectors place the partition on its device.
	start, size, first, sectors int64
	guid                        [16]byte
	data                        []byte
}

func (p partition) inGPT() bool { return p.typ != typeGP1 && p.typ != typeGPT }

// layout is NVIDIA's partition layout for the boot device (boot0 and boot1
// as one space) and the user area, placed on one module's eMMC.
type layout struct {
	devices  [2]device
	diskGUID [16]byte
	storage  Storage
}

type device struct {
	instance uint32
	parts    []partition
}

func (l *layout) boot() []partition { return l.devices[0].parts }
func (l *layout) user() []partition { return l.devices[1].parts }

// newLayout parses the kit's layout template and places it on s, drawing a
// GUID for each partition in layout order and then one for the disk.
func newLayout(template []byte, s Storage, random io.Reader) (*layout, error) {
	var doc struct {
		Devices []struct {
			Type       string `xml:"type,attr"`
			Instance   uint32 `xml:"instance,attr"`
			Partitions []struct {
				Name  string `xml:"name,attr"`
				Type  string `xml:"type,attr"`
				Start string `xml:"start_location"`
				Size  string `xml:"size"`
				Attr  string `xml:"allocation_attribute"`
				File  string `xml:"filename"`
			} `xml:"partition"`
		} `xml:"device"`
	}
	unfamiliar := func(why string, args ...any) error {
		return fmt.Errorf("the kit's partition layout %s; it is only proven for flash_l4t_t210_emmc_p3448.xml from L4T R32.7.6", fmt.Sprintf(why, args...))
	}
	if err := xml.Unmarshal([]byte(layoutTags.Replace(string(template))), &doc); err != nil {
		return nil, unfamiliar("does not parse (%v)", err)
	}
	if len(doc.Devices) != 2 {
		return nil, unfamiliar("has %d devices, not a boot device and a user area", len(doc.Devices))
	}

	l := &layout{storage: s}
	for d, dev := range doc.Devices {
		if dev.Type != "sdmmc" {
			return nil, unfamiliar("has a %q device", dev.Type)
		}
		l.devices[d].instance = dev.Instance
		var id uint32
		for _, x := range dev.Partitions {
			p := partition{name: x.Name, file: strings.TrimSpace(x.File)}
			var ok bool
			if p.typ, ok = partitionTypes[x.Type]; !ok {
				return nil, unfamiliar("gives %s the unknown type %q", x.Name, x.Type)
			}
			if p.inGPT() {
				id++
				p.id = id
			}
			start, err1 := strconv.ParseInt(cmp.Or(strings.TrimSpace(x.Start), "0"), 0, 64)
			size, err2 := strconv.ParseInt(strings.TrimSpace(x.Size), 0, 64)
			attr, err3 := strconv.ParseUint(strings.TrimSpace(x.Attr), 0, 32)
			if err1 != nil || err2 != nil || err3 != nil || (size != -1 && (size <= 0 || size%sector != 0)) {
				return nil, unfamiliar("gives %s an unreadable start, size or attribute", x.Name)
			}
			p.start, p.size, p.attr = start, size, uint32(attr)
			guid, err := newGUID(random)
			if err != nil {
				return nil, err
			}
			p.guid = guid
			l.devices[d].parts = append(l.devices[d].parts, p)
		}
	}
	var err error
	if l.diskGUID, err = newGUID(random); err != nil {
		return nil, err
	}
	if err := l.place(); err != nil {
		return nil, err
	}
	return l, nil
}

// newGUID draws a version 4 GUID, stamped in the mixed-endian byte order GPT
// stores GUIDs in.
func newGUID(random io.Reader) ([16]byte, error) {
	var g [16]byte
	if _, err := io.ReadFull(random, g[:]); err != nil {
		return g, fmt.Errorf("generating a partition GUID failed: %w", err)
	}
	g[7] = g[7]&0x0F | 0x40
	g[8] = g[8]&0x3F | 0x80
	return g, nil
}

// place puts the boot device's partitions one after another, except where
// the layout gives a start, and the user area's one after another from the
// end of the primary GPT, unaligned, as NVIDIA's tools do.
func (l *layout) place() error {
	var at int64
	for i := range l.boot() {
		p := &l.boot()[i]
		if p.start != 0 {
			at = p.start
		}
		p.first, p.sectors = at/sector, p.size/sector
		at += p.size
		if p.first+p.sectors > l.storage.BootSectors {
			return fmt.Errorf("the eMMC's boot device is %d sectors, too small for NVIDIA's layout to fit %s; the layout is for the Jetson Nano 16GB eMMC module (P3448-0002), so check that is the module attached", l.storage.BootSectors, p.name)
		}
	}

	lba, last := int64(gptFirstLBA), l.storage.UserSectors-gptFirstLBA
	for i := range l.user() {
		p := &l.user()[i]
		if !p.inGPT() {
			continue
		}
		p.first, p.sectors = lba, p.size/sector
		if p.attr&attrRest != 0 {
			p.sectors = max(last-lba+1, 0)
		}
		lba += p.sectors
		if p.sectors < p.size/sector || lba-1 > last {
			return fmt.Errorf("the eMMC's user area is %d sectors, too small for NVIDIA's layout to fit %s; the layout is for the Jetson Nano 16GB eMMC module (P3448-0002), so check that is the module attached", l.storage.UserSectors, p.name)
		}
	}
	return nil
}

// partitionFiles builds what each layout filename puts in its partitions,
// all but the partition table.
func partitionFiles(in Inputs, bc BootChain, now time.Time) (map[string][]byte, error) {
	tbc, err := signHeadered("nvtboot_cpu.bin", in.NVTBootCPU)
	if err != nil {
		return nil, err
	}
	wb0, err := signWB0(in.WarmBoot)
	if err != nil {
		return nil, err
	}
	tos, err := signTOS(in.TOS)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		"nvtboot.bin":           in.NVTBoot,
		"nvtboot_cpu.bin":       tbc,
		dtbFile:                 bc.DTB,
		"cboot.bin":             bc.CBoot,
		"warmboot.bin":          wb0,
		"sc7entry-firmware.bin": wrap("SC7F", in.SC7Entry, sc7EntryLoad),
		"emmc_bootblob_ver.txt": versionFile(now),
		appFile:                 nil,
		"tos-mon-only.img":      tos,
		"eks.img":               in.EKS,
		"boot.img":              signLNX(in.UBoot),
		"bmp.blob":              in.BMP,
		"rp4.blob":              in.RP4,
	}, nil
}

func (l *layout) attach(files map[string][]byte) error {
	for d := range l.devices {
		for i := range l.devices[d].parts {
			p := &l.devices[d].parts[i]
			if p.file == "" {
				continue
			}
			data, ok := files[p.file]
			if !ok {
				return fmt.Errorf("the kit's partition layout puts %s in %s, which gosd does not build; it is only proven for flash_l4t_t210_emmc_p3448.xml from L4T R32.7.6", p.file, p.name)
			}
			if int64(len(data)) > p.size {
				return fmt.Errorf("%s is %d bytes, too big for its %d-byte %s partition; the kit is damaged or is not R32.7.6, so download it again", p.file, len(data), p.size, p.name)
			}
			p.data = data
		}
	}
	return nil
}

// writes lists the partitions with something to write in the order NVIDIA's
// flash writes them: the partition tables, then the rest in layout order.
func (l *layout) writes() []Partition {
	var tables, rest []Partition
	for _, p := range slices.Concat(l.boot(), l.user()) {
		switch {
		case p.file == "":
		case p.typ == typePartitionTable:
			tables = append(tables, Partition{Name: p.name, Data: p.data})
		default:
			rest = append(rest, Partition{Name: p.name, Data: p.data})
		}
	}
	return append(tables, rest...)
}

const ptEntryLen = 88

// partitionTable is the PT partitions' binary layout. NVIDIA's tool dumps its
// in-memory structs there, 32-bit host heap pointers and all; the device
// ignores those words, so this writes the offset of what each points to.
func (l *layout) partitionTable() ([]byte, error) {
	const sdmmc = 2
	b := make([]byte, 12+16*len(l.devices))
	le().PutUint32(b[0x0:], 10000)
	le().PutUint32(b[0x4:], uint32(len(l.devices)))
	le().PutUint32(b[0x8:], 12)
	for d, dev := range l.devices {
		rec := 12 + 16*d
		le().PutUint32(b[rec:], sdmmc)
		le().PutUint32(b[rec+4:], dev.instance)
		le().PutUint32(b[rec+8:], uint32(len(dev.parts)))
		le().PutUint32(b[rec+12:], uint32(len(b)))

		entries := len(b)
		b = append(b, make([]byte, ptEntryLen*len(dev.parts))...)
		for i, p := range dev.parts {
			nameAt, fileAt := len(b), 0
			b = append(append(b, p.name...), 0)
			if p.file != "" {
				fileAt = len(b)
			}
			b = append(append(b, p.file...), 0)

			e := b[entries+ptEntryLen*i:]
			le().PutUint32(e[0x00:], uint32(nameAt))
			le().PutUint32(e[0x04:], p.id)
			le().PutUint32(e[0x08:], p.typ)
			le().PutUint32(e[0x0C:], 3)
			le().PutUint32(e[0x10:], 1)
			le().PutUint64(e[0x14:], uint64(p.start))
			le().PutUint64(e[0x1C:], uint64(p.size))
			le().PutUint32(e[0x2C:], p.attr)
			copy(e[0x3C:], p.guid[:])
			le().PutUint32(e[0x54:], uint32(fileAt))
		}
	}

	pt := l.boot()[slices.IndexFunc(l.boot(), func(p partition) bool { return p.typ == typePartitionTable })]
	if int64(len(b)) > pt.size-16 {
		return nil, fmt.Errorf("the partition table is %d bytes, too big for its %d-byte partition; it is only proven for flash_l4t_t210_emmc_p3448.xml from L4T R32.7.6", len(b), pt.size)
	}
	out := make([]byte, pt.size)
	copy(out, b)
	trailer := out[len(out)-16:]
	trailer[0] = 1 // version 1.0
	copy(trailer[8:], "PTHD")
	le().PutUint32(trailer[12:], crc32.ChecksumIEEE(out[:len(out)-4]))
	return out, nil
}

// basicData is the GPT type GUID EBD0A0A2-B9E5-4433-87C0-68B6B72699C7.
var basicData = [16]byte{0xA2, 0xA0, 0xD0, 0xEB, 0xE5, 0xB9, 0x33, 0x44, 0x87, 0xC0, 0x68, 0xB6, 0xB7, 0x26, 0x99, 0xC7}

// gpt is the user area's primary GPT as it sits from LBA 1: the header
// sector, then the entries. Each entry reuses its partition-table GUID.
func (l *layout) gpt() []byte {
	b := make([]byte, sector+gptEntries*gptEntryLen)
	entries := b[sector:]
	n := 0
	for _, p := range l.user() {
		if !p.inGPT() {
			continue
		}
		e := entries[n*gptEntryLen:]
		copy(e, basicData[:])
		copy(e[16:], p.guid[:])
		le().PutUint64(e[32:], uint64(p.first))
		le().PutUint64(e[40:], uint64(p.first+p.sectors-1))
		for i, c := range utf16.Encode([]rune(p.name)) {
			le().PutUint16(e[56+2*i:], c)
		}
		n++
	}

	userSectors := uint64(l.storage.UserSectors)
	h := b[:sector]
	copy(h, "EFI PART")
	le().PutUint32(h[8:], 0x10000)
	le().PutUint32(h[12:], 92)
	le().PutUint64(h[24:], 1)
	le().PutUint64(h[32:], userSectors-1)
	le().PutUint64(h[40:], gptFirstLBA)
	// One sector more than the secondary GPT leaves free, as NVIDIA writes it.
	le().PutUint64(h[48:], userSectors-33)
	copy(h[56:], l.diskGUID[:])
	le().PutUint64(h[72:], 2)
	le().PutUint32(h[80:], gptEntries)
	le().PutUint32(h[84:], gptEntryLen)
	le().PutUint32(h[88:], crc32.ChecksumIEEE(entries))
	le().PutUint32(h[16:], crc32.ChecksumIEEE(h[:92]))
	return b
}

// versionFile is the VER partitions' text. Its board line is fixed: NVIDIA
// reads FAB from each module's EEPROM, but nothing on the boot path is known
// to read this file.
func versionFile(now time.Time) []byte {
	body := "NV3\n# R32 , REVISION: 7.6\nBOARDID=3448 BOARDSKU=0002 FAB=401\n" + now.Format("20060102150405") + "\n"
	return fmt.Appendf(nil, "%sBYTES:%d CRC32:%d\n", body, len(body), cksum([]byte(body)))
}

// cksum is POSIX cksum(1)'s CRC: CRC-32 fed most significant bit first over
// the data and then its length's significant bytes, low byte first, then
// complemented. hash/crc32 only does the bit-reversed form.
func cksum(b []byte) uint32 {
	var crc uint32
	add := func(c byte) {
		crc ^= uint32(c) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}
	for _, c := range b {
		add(c)
	}
	for n := len(b); n > 0; n >>= 8 {
		add(byte(n))
	}
	return ^crc
}

const (
	bctBootloadersAt = 0x232C
	bctBootloaderLen = 0x12C
	bctBlockLog2     = 14
	bctPageLog2      = 9
	nvtbootLoad      = 0x40010000

	bctKFSAt = 0x480
	bctBFSAt = 0x4C0
)

// finalBCT adds what a flashed BCT carries beyond the recovery one: a signed
// entry for each copy of NVC, then unsigned summaries of the boot files (BFS)
// and kernel files (KFS) each copy boots with.
func (l *layout) finalBCT(recovery []byte) ([]byte, error) {
	b := bytes.Clone(recovery)
	var nvc []int
	for i, p := range l.boot() {
		if p.name == "NVC" || strings.HasPrefix(p.name, "NVC-") {
			nvc = append(nvc, i)
		}
	}
	le().PutUint32(b[bctBootloadersAt:], uint32(len(nvc)))
	for n, i := range nvc {
		p := l.boot()[i]
		e := b[bctBootloadersAt+4+n*bctBootloaderLen:]
		off := p.first * sector
		le().PutUint32(e[0x00:], 1)
		le().PutUint32(e[0x04:], uint32(off>>bctBlockLog2))
		le().PutUint32(e[0x08:], uint32(off&(1<<bctBlockLog2-1)>>bctPageLog2))
		le().PutUint32(e[0x0C:], uint32(len(p.data)))
		le().PutUint32(e[0x10:], nvtbootLoad)
		le().PutUint32(e[0x14:], nvtbootLoad)
		le().PutUint32(e[0x18:], p.id)
		sum := cmac(zeroKey, p.data)
		copy(e[0x1C:], sum[:])
	}
	sum := cmac(zeroKey, b[bctSignedFrom:bctSignedTo])
	copy(b[bctCMACAt:], sum[:])

	// A boot-file set is every boot partition after one NVC, up to the next.
	for n, i := range nvc {
		end := len(l.boot())
		if n+1 < len(nvc) {
			end = nvc[n+1]
		}
		set := l.boot()[i+1 : end]
		putFileSet(b, bctBFSAt, n, set, set[0].first, false)
		le().PutUint32(b[bctBFSAt+0x30+4*n:], uint32(set[0].size))
	}
	for n, suffix := range []string{"", "-1"} {
		var set []partition
		for _, name := range []string{"DTB", "TOS", "EKS", "LNX"} {
			i := slices.IndexFunc(l.user(), func(p partition) bool { return p.name == name+suffix })
			if i < 0 {
				return nil, fmt.Errorf("the kit's partition layout has no %s partition; it is only proven for flash_l4t_t210_emmc_p3448.xml from L4T R32.7.6", name+suffix)
			}
			set = append(set, l.user()[i])
		}
		// The BCT addresses the user area as following the boot device.
		putFileSet(b, bctKFSAt, n, set, l.storage.BootSectors+set[0].first, true)
	}
	return b, nil
}

// putFileSet records file set n's length, SUM and first sector in the
// arrays at base. The length counts each partition whole, except a kernel
// set's last, which counts only its file. The SUM is the XOR of the set's
// words, each file zero-padded to its partition; partitions are whole
// sectors, so that is the XOR of each file's own sum.
func putFileSet(b []byte, base, n int, set []partition, first int64, kernel bool) {
	var length int64
	var sum uint32
	for i, p := range set {
		if kernel && i == len(set)-1 {
			length += int64(len(p.data))
		} else {
			length += p.size
		}
		sum ^= xor32(p.data)
	}
	le().PutUint32(b[base+4*n:], uint32(length))
	le().PutUint32(b[base+0x10+4*n:], sum)
	le().PutUint32(b[base+0x20+4*n:], uint32(first))
}

// xor32 XORs b's little-endian 32-bit words, zero-padding a partial last
// word.
func xor32(b []byte) uint32 {
	var x uint32
	for ; len(b) >= 4; b = b[4:] {
		x ^= le().Uint32(b)
	}
	var tail [4]byte
	copy(tail[:], b)
	return x ^ le().Uint32(tail[:])
}
