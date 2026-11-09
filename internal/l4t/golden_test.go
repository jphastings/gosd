package l4t

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var readRealKit = sync.OnceValues(func() (Inputs, error) {
	f, err := os.Open(os.Getenv("GOSD_L4T_KIT"))
	if err != nil {
		return Inputs{}, err
	}
	defer func() { _ = f.Close() }()
	return R32_7_6.Read(f)
})

func realKit(t *testing.T) Inputs {
	t.Helper()
	if os.Getenv("GOSD_L4T_KIT") == "" {
		t.Skip("GOSD_L4T_KIT not set")
	}
	in, err := readRealKit()
	if err != nil {
		t.Fatal(err)
	}
	return in
}

// Set GOSD_L4T_KIT to a real Jetson-210_Linux_R32.7.6_aarch64.tbz2 to prove
// the output matches the files that booted a Jetson Nano on hardware.
func TestGoldenAgainstTheRealKit(t *testing.T) {
	bc, err := Build(realKit(t))
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		got  []byte
		want string
	}{
		"rcm_0": {bc.RCM0, "9eb594faa8f13d1d99368febc15fdf880e744e53716e662de86141ecc81403df"},
		"rcm_1": {bc.RCM1, "5a299e8df8eccc5db2596f8492e7744ebae99473b2c915cbb1daa279e907c643"},
		"bct":   {bc.bct, "f7e6362074bcb8d4408ce2fbf497ceb947b765884b16b69f919bb12fbb84e0ea"},
		"cboot": {bc.CBoot, "9a0a80e58da44538d35a7aa15334f6e8ba9eae97b4a37899de7716a437194f22"},
		"dtb":   {bc.DTB, "af297f9c8e191dff7f50dcd686f3d8191f9d7fa00b8386c0ba25fea79fffc075"},
	} {
		sum := sha256.Sum256(c.got)
		if got := hex.EncodeToString(sum[:]); got != c.want {
			t.Errorf("%s sha256 = %s, want %s", name, got, c.want)
		}
	}
}

// nvidiaFiles names the file NVIDIA's flash wrote to each partition (and its
// "-1" copy).
var nvidiaFiles = map[string]string{
	"PT": "crc-flash.xml.bin", "NVC": "nvtboot.bin", "TBC": "nvtboot_cpu.bin.encrypt",
	"RP1": dtbFile + ".encrypt", "EBT": "cboot.bin.encrypt", "WB0": "warmboot.bin.encrypt",
	"BPF": "sc7entry-firmware.bin.encrypt", "VER_b": "emmc_bootblob_ver.txt", "VER": "emmc_bootblob_ver.txt",
	"DTB": dtbFile + ".encrypt", "TOS": "tos-mon-only.img.encrypt", "EKS": "eks.img",
	"LNX": "boot.img.encrypt", "BMP": "bmp.blob", "RP4": "rp4.blob",
}

// Set GOSD_L4T_BLOB as well, to the directory of files NVIDIA's own tools
// wrote for a first flash of a Jetson Nano 16GB eMMC module, to prove a first
// flash identical to them. NVIDIA's GUIDs are already version 4, so drawing
// them again reproduces them.
func TestFirstFlashMatchesNVIDIAs(t *testing.T) {
	in := realKit(t)
	dir := os.Getenv("GOSD_L4T_BLOB")
	if dir == "" {
		t.Skip("GOSD_L4T_BLOB not set")
	}
	nvidia := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	nvPT, nvGPT := nvidia(ptFile), nvidia("gpt.bin")
	s := Storage{BootSectors: 16384, UserSectors: int64(binary.LittleEndian.Uint64(nvGPT[32:])) + 1}
	stamp := regexp.MustCompile(`(?m)^\d{14}$`).Find(nvidia("emmc_bootblob_ver.txt"))
	when, err := time.Parse("20060102150405", string(stamp))
	if err != nil {
		t.Fatal(err)
	}

	fresh, err := BuildFirstFlash(in, s, when, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	entries, pointers := walkPartitionTable(fresh.Partitions[0].Data)
	var draws []byte
	for _, e := range entries {
		draws = append(draws, nvPT[e+0x3C:e+0x4C]...)
	}
	draws = append(draws, nvGPT[56:72]...)

	ff, err := BuildFirstFlash(in, s, when, bytes.NewReader(draws))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ff.GPT, nvGPT) {
		t.Error("gpt.bin differs from NVIDIA's")
	}
	for _, p := range ff.Partitions {
		switch {
		case p.Name == "APP":
			if p.Data != nil {
				t.Error("APP has data")
			}
		case p.Name == "PT" || p.Name == "PT-1":
			// NVIDIA's pointer words are host heap addresses: take them, and
			// the CRC over them, and the rest must match.
			if len(pointers) != 81 {
				t.Errorf("the partition table has %d pointer words, want 81", len(pointers))
			}
			pt := bytes.Clone(p.Data)
			for _, o := range pointers {
				copy(pt[o:o+4], nvPT[o:o+4])
			}
			binary.LittleEndian.PutUint32(pt[len(pt)-4:], crc32.ChecksumIEEE(pt[:len(pt)-4]))
			if !bytes.Equal(pt, nvPT) {
				t.Errorf("%s differs from NVIDIA's beyond its pointer words", p.Name)
			}
		case !bytes.Equal(p.Data, nvidia(nvidiaFiles[strings.TrimSuffix(p.Name, "-1")])):
			t.Errorf("%s differs from NVIDIA's", p.Name)
		}
	}

	// The BCT's boot-file sums cover the partition table, so it can only match
	// NVIDIA's when built over NVIDIA's.
	bc, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	l, err := newLayout(in.Layout, s, bytes.NewReader(draws))
	if err != nil {
		t.Fatal(err)
	}
	files, err := partitionFiles(in, bc, when)
	if err != nil {
		t.Fatal(err)
	}
	files[ptFile] = nvPT
	if err := l.attach(files); err != nil {
		t.Fatal(err)
	}
	bct, err := l.finalBCT(bc.bct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bct, nvidia("P3448_A00_lpddr4_204Mhz_P987.bct")) {
		t.Error("the BCT differs from NVIDIA's")
	}
}
