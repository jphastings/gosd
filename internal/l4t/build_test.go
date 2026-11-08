package l4t

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

func syntheticConfig(mutate func(set, field int, line string) string) []byte {
	var b strings.Builder
	b.WriteString("# CFG Version 13\n")
	for s := 0; s < sdramSets; s++ {
		for f := 0; f < sdramFields; f++ {
			line := fmt.Sprintf("SDRAM[%d].Field%d = 0x%08x;", s, f, s*1000+f)
			if f == 0 {
				line = fmt.Sprintf("SDRAM[%d].MemoryType = NvBootMemoryType_LpDdr4;", s)
			}
			if mutate != nil {
				line = mutate(s, f, line)
			}
			b.WriteString(line + "\n")
		}
	}
	return []byte(b.String())
}

func syntheticInputs() Inputs {
	return Inputs{
		Recovery:  bytes.Repeat([]byte{7}, 5000),
		CBoot:     bytes.Repeat([]byte{9}, 0x400+100),
		BCTConfig: syntheticConfig(nil),
		DTB:       []byte("fake-dtb"),

		NVTBoot:    bytes.Repeat([]byte{1}, 1000),
		NVTBootCPU: bytes.Repeat([]byte{2}, 0x400+64),
		WarmBoot:   append(make([]byte, 0x240), bytes.Repeat([]byte{3}, 64)...),
		SC7Entry:   []byte("sc7"),
		TOS:        bytes.Repeat([]byte{4}, 32),
		EKS:        []byte("eks"),
		BMP:        []byte("bmp"),
		RP4:        []byte("rp4"),
		UBoot:      []byte("u-boot"),
		Layout:     syntheticLayout(),
	}
}

func assertSigned(t *testing.T, name string, file []byte, cmacAt, signedFrom, signedTo int) {
	t.Helper()
	want := cmac(zeroKey, file[signedFrom:signedTo])
	if !bytes.Equal(file[cmacAt:cmacAt+16], want[:]) {
		t.Errorf("%s: CMAC at %#x does not cover [%#x:%#x]", name, cmacAt, signedFrom, signedTo)
	}
}

func TestBuildSignsEveryFile(t *testing.T) {
	in := syntheticInputs()
	bc, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}

	if len(bc.RCM0) != 1032 {
		t.Errorf("rcm_0 is %d bytes, want 1032", len(bc.RCM0))
	}
	for name, rcm := range map[string][]byte{"rcm_0": bc.RCM0, "rcm_1": bc.RCM1} {
		if len(rcm)%16 != 8 {
			t.Errorf("%s length %d %% 16 = %d, want 8", name, len(rcm), len(rcm)%16)
		}
		if got := binary.LittleEndian.Uint32(rcm); int(got) != len(rcm) {
			t.Errorf("%s declares length %d, is %d", name, got, len(rcm))
		}
		assertSigned(t, name, rcm, 0x128, 0x238, len(rcm))
	}
	if !bytes.Equal(bc.RCM1[rcmHeaderLen:rcmHeaderLen+len(in.Recovery)], in.Recovery) {
		t.Error("rcm_1 does not carry the recovery applet")
	}

	assertSigned(t, "cboot", bc.CBoot, 0x118, 0x230, 0x230+(len(bc.CBoot)-0x230)/16*16)
	if got := binary.LittleEndian.Uint64(bc.CBoot[0x8:]); int(got) != len(bc.CBoot) {
		t.Errorf("cboot declares length %d, is %d", got, len(bc.CBoot))
	}

	assertSigned(t, "dtb", bc.DTB, 0x118, 0x230, len(bc.DTB))
	if len(bc.DTB) != 0x400+16 || string(bc.DTB[4:8]) != "DTB\x00" || bc.DTB[0x400+len(in.DTB)] != 0x80 {
		t.Error("dtb is not a padded DTB-headed image")
	}

	if len(bc.BCT) != bctLen {
		t.Fatalf("BCT is %d bytes, want %d", len(bc.BCT), bctLen)
	}
	assertSigned(t, "bct", bc.BCT, 0x310, 0x510, 0x2800)
}

func TestBuildPlacesSDRAMValuesInFileOrder(t *testing.T) {
	bc, err := Build(syntheticInputs())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ set, field int }{{0, 1}, {1, 0}, {2, 473}, {3, 473}} {
		at := 0x58C + (c.set*474+c.field)*4
		want := uint32(c.set*1000 + c.field)
		if c.field == 0 {
			want = 3
		}
		if got := binary.LittleEndian.Uint32(bc.BCT[at:]); got != want {
			t.Errorf("SDRAM[%d] field %d at %#x = %#x, want %#x", c.set, c.field, at, got, want)
		}
	}
}

func TestBuildRejectsAnUnfamiliarConfig(t *testing.T) {
	for name, mutate := range map[string]func(s, f int, line string) string{
		"other memory type": func(s, f int, l string) string {
			return strings.Replace(l, "NvBootMemoryType_LpDdr4", "NvBootMemoryType_Ddr3", 1)
		},
		"renamed field": func(s, f int, l string) string {
			if s == 2 && f == 5 {
				return "SDRAM[2].Surprise = 0x00000001;"
			}
			return l
		},
		"missing field": func(s, f int, l string) string {
			if s == 3 && f == 473 {
				return ""
			}
			return l
		},
		"bad hex": func(s, f int, l string) string {
			if f == 7 {
				return fmt.Sprintf("SDRAM[%d].Field7 = 0xZZ;", s)
			}
			return l
		},
	} {
		in := syntheticInputs()
		in.BCTConfig = syntheticConfig(mutate)
		if _, err := Build(in); err == nil {
			t.Errorf("%s: config was accepted", name)
		}
	}
}
