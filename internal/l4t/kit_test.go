package l4t

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path"
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

func TestReadExtractsEveryMember(t *testing.T) {
	kit, raw := fixtureKit(t, "kit.tbz2")
	in, err := kit.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	// Each fixture member holds its own file name.
	for name, data := range in.members() {
		if string(*data) != path.Base(name) {
			t.Errorf("%s = %q", name, *data)
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
