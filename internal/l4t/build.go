package l4t

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// BootChain is the five signed files the Tegra X1 boot ROM and its recovery
// applet need to start CBoot's flashing server on a Jetson Nano.
type BootChain struct {
	RCM0, RCM1, BCT, CBoot, DTB []byte
}

// The module has production fuses and no SBK, so everything is signed with
// an all-zero AES-CMAC key and nothing is encrypted.
var zeroKey [16]byte

// Build signs the kit's raw files into a BootChain. The BCT is built without
// bootloader entries, which booting into recovery does not need.
func Build(in Inputs) (BootChain, error) {
	if len(in.CBoot) < blHeaderLen {
		return BootChain{}, fmt.Errorf("cboot.bin is %d bytes, shorter than its %d-byte signature header; the kit member is damaged, so download it again", len(in.CBoot), blHeaderLen)
	}
	bct, err := buildBCT(in.BCTConfig)
	if err != nil {
		return BootChain{}, err
	}
	return BootChain{
		RCM0:  rcm(opcodeNoPayload, nil, 0),
		RCM1:  rcm(opcodeRecoveryApplet, in.Recovery, recoveryLoadAddress),
		BCT:   bct,
		CBoot: signCBoot(in.CBoot),
		DTB:   signDTB(in.DTB),
	}, nil
}

const (
	opcodeNoPayload      = 6
	opcodeRecoveryApplet = 4
	recoveryLoadAddress  = 0x40010000

	rcmHeaderLen  = 0x2A8
	rcmMinLen     = 1024
	rcmSignedFrom = 0x238
	rcmCMACAt     = 0x128

	blSignedFrom = 0x230
	blCMACAt     = 0x118
	blHeaderLen  = 0x400

	unspecified = 0xFEEDDEADBEEFBAB1
)

func le() binary.ByteOrder { return binary.LittleEndian }

func putCMAC(b []byte, at, signedFrom int) {
	// The CMAC covers whole 16-byte blocks only; the formats pad to that.
	signed := b[signedFrom:]
	sum := cmac(zeroKey, signed[:len(signed)/16*16])
	copy(b[at:], sum[:])
}

// rcm builds a recovery-mode message: header, payload, then padding so the
// length is at least 1024 and the payload area is a multiple of 16 bytes.
func rcm(opcode uint32, payload []byte, arg0 uint32) []byte {
	n := rcmHeaderLen + len(payload)
	pad := 0
	if n < rcmMinLen {
		pad = rcmMinLen - n
	}
	n += pad
	pad += 16 - (n-rcmHeaderLen)&0xF
	total := rcmHeaderLen + len(payload) + pad

	msg := make([]byte, total)
	le().PutUint32(msg[0:], uint32(total))
	le().PutUint32(msg[0x258:], opcode)
	le().PutUint32(msg[0x25C:], uint32(total))
	le().PutUint32(msg[0x260:], uint32(len(payload)))
	le().PutUint32(msg[0x264:], 0x00210001)
	le().PutUint32(msg[0x268:], arg0)
	msg[0x2A0] = 0x80
	copy(msg[rcmHeaderLen:], payload)
	msg[rcmHeaderLen+len(payload)] = 0x80
	putCMAC(msg, rcmCMACAt, rcmSignedFrom)
	return msg
}

// signCBoot signs cboot.bin, whose first 0x400 bytes already reserve the
// signature header.
func signCBoot(raw []byte) []byte {
	img := bytes.Clone(raw)
	le().PutUint64(img[0x8:], uint64(len(img)))
	le().PutUint64(img[0x240:], uint64(len(img)))
	putCMAC(img, blCMACAt, blSignedFrom)
	return img
}

func signDTB(raw []byte) []byte {
	padded := len(raw) + 1 + (-(len(raw) + 1))&0xF
	total := blHeaderLen + padded

	img := make([]byte, total)
	copy(img[4:], "DTB\x00")
	le().PutUint64(img[0x8:], uint64(total))
	le().PutUint64(img[0x10:], unspecified)
	fill(img[0x18:0x118], 0xAA)
	fill(img[0x118:0x228], 0xEE)
	fill(img[0x230:0x240], 0xCC)
	le().PutUint64(img[0x240:], uint64(total))
	le().PutUint64(img[0x248:], unspecified)
	le().PutUint64(img[0x250:], unspecified)
	copy(img[blHeaderLen:], raw)
	img[blHeaderLen+len(raw)] = 0x80
	putCMAC(img, blCMACAt, blSignedFrom)
	return img
}

func fill(b []byte, v byte) {
	for i := range b {
		b[i] = v
	}
}

const (
	bctLen          = 10240
	bctSDRAMAt      = 0x58C
	bctCMACAt       = 0x310
	bctSignedFrom   = 0x510
	bctSignedTo     = 0x2800
	sdramSets       = 4
	sdramFields     = 474
	memoryTypeLpDDR = 3
)

// signedBCTWords are the words NVIDIA's tools write from 0x530 for this
// module; the SDRAM sets follow at 0x58C.
var signedBCTWords = [8]uint32{0x00210001, 14, 9, 0x1000000, 1, 4, 9, 2}

// buildBCT lays the config's SDRAM sets over the constants NVIDIA's tools
// write for the Nano (ODMDATA 0xA4000 among them). The layout is only proven
// for the P3448 LPDDR4 204MHz config, so anything unfamiliar is refused.
func buildBCT(cfg []byte) ([]byte, error) {
	sets, err := parseSDRAM(cfg)
	if err != nil {
		return nil, err
	}

	b := make([]byte, bctLen)
	le().PutUint32(b[0x0:], 0x200)
	le().PutUint32(b[0x4:], 0xE0F)
	le().PutUint32(b[0x47C:], 3)
	le().PutUint32(b[0x508:], 0xA4000)
	for i, w := range signedBCTWords {
		le().PutUint32(b[0x530+4*i:], w)
	}
	for s, set := range sets {
		for n, v := range set {
			le().PutUint32(b[bctSDRAMAt+(s*sdramFields+n)*4:], v)
		}
	}
	b[0x27EC] = 0x80
	sum := cmac(zeroKey, b[bctSignedFrom:bctSignedTo])
	copy(b[bctCMACAt:], sum[:])
	return b, nil
}

func parseSDRAM(cfg []byte) ([sdramSets][]uint32, error) {
	var sets [sdramSets][]uint32
	var names []string
	sc := bufio.NewScanner(bytes.NewReader(cfg))
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(text, "SDRAM[") {
			continue
		}
		fail := func(why string) error {
			return fmt.Errorf("BCT config line %d (%q): %s; this layout is only proven for NVIDIA's P3448_A00_lpddr4_204Mhz_P987.cfg from L4T R32.7.6", line, text, why)
		}
		key, val, ok := strings.Cut(strings.TrimSuffix(text, ";"), "=")
		if !ok || !strings.HasSuffix(text, ";") {
			return sets, fail("not a 'name = value;' assignment")
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		setStr, field, ok := strings.Cut(strings.TrimPrefix(key, "SDRAM["), "].")
		set, err := strconv.Atoi(setStr)
		if !ok || err != nil || set < 0 || set >= sdramSets {
			return sets, fail("expected SDRAM[0] to SDRAM[3]")
		}

		var v uint32
		switch {
		case val == "NvBootMemoryType_LpDdr4":
			v = memoryTypeLpDDR
		case strings.HasPrefix(val, "0x"):
			n, err := strconv.ParseUint(val[2:], 16, 32)
			if err != nil {
				return sets, fail("value is not a 32-bit hex number")
			}
			v = uint32(n)
		default:
			return sets, fail("value is neither hex nor NvBootMemoryType_LpDdr4")
		}

		if set == 0 {
			names = append(names, field)
		} else if i := len(sets[set]); i >= len(names) || names[i] != field {
			return sets, fail(fmt.Sprintf("field does not match SDRAM[0]'s field %d", i))
		}
		sets[set] = append(sets[set], v)
	}
	for s, set := range sets {
		if len(set) != sdramFields {
			return sets, fmt.Errorf("BCT config has %d fields in SDRAM[%d], want %d; this layout is only proven for NVIDIA's P3448_A00_lpddr4_204Mhz_P987.cfg from L4T R32.7.6", len(set), s, sdramFields)
		}
	}
	return sets, nil
}
