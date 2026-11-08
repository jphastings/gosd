package l4t

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"
)

// Set GOSD_L4T_KIT to a real Jetson-210_Linux_R32.7.6_aarch64.tbz2 to prove
// the output matches the files that booted a Jetson Nano on hardware.
func TestGoldenAgainstTheRealKit(t *testing.T) {
	path := os.Getenv("GOSD_L4T_KIT")
	if path == "" {
		t.Skip("GOSD_L4T_KIT not set")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	start := time.Now()
	in, err := R32_7_6.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Read took %s", time.Since(start))

	bc, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		got  []byte
		want string
	}{
		"rcm_0": {bc.RCM0, "9eb594faa8f13d1d99368febc15fdf880e744e53716e662de86141ecc81403df"},
		"rcm_1": {bc.RCM1, "5a299e8df8eccc5db2596f8492e7744ebae99473b2c915cbb1daa279e907c643"},
		"bct":   {bc.BCT, "f7e6362074bcb8d4408ce2fbf497ceb947b765884b16b69f919bb12fbb84e0ea"},
		"cboot": {bc.CBoot, "9a0a80e58da44538d35a7aa15334f6e8ba9eae97b4a37899de7716a437194f22"},
		"dtb":   {bc.DTB, "af297f9c8e191dff7f50dcd686f3d8191f9d7fa00b8386c0ba25fea79fffc075"},
	} {
		sum := sha256.Sum256(c.got)
		if got := hex.EncodeToString(sum[:]); got != c.want {
			t.Errorf("%s sha256 = %s, want %s", name, got, c.want)
		}
	}
}
