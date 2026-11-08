package l4t

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func fixtureKit(t *testing.T, name string) (Kit, []byte) {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return Kit{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(raw))}, raw
}

func TestReadExtractsTheFourMembers(t *testing.T) {
	kit, raw := fixtureKit(t, "kit.tbz2")
	in, err := kit.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{string(in.Recovery), string(in.CBoot), string(in.BCTConfig), string(in.DTB)}
	want := []string{"recovery-applet", "cboot", "config", "dtb"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("member %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadRefusesAWrongPin(t *testing.T) {
	kit, raw := fixtureKit(t, "kit.tbz2")
	if _, err := kit.Read(bytes.NewReader(raw[:len(raw)-1])); err == nil {
		t.Error("a truncated kit was accepted")
	}

	kit.SHA256 = strings.Repeat("0", 64)
	if _, err := kit.Read(bytes.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "not R32.7.6") {
		t.Errorf("want a hash-mismatch error, got %v", err)
	}
}

func TestReadNamesAMissingMember(t *testing.T) {
	kit, raw := fixtureKit(t, "kit-missing-dtb.tbz2")
	_, err := kit.Read(bytes.NewReader(raw))
	if err == nil || !strings.Contains(err.Error(), "tegra210-p3448-0002-p3449-0000-b00.dtb") {
		t.Errorf("want an error naming the DTB, got %v", err)
	}
}
