package l4t

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
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
	// Each fixture member holds its own file name.
	pins := map[string]string{}
	for member := range R32_7_6.Members {
		sum := sha256.Sum256([]byte(path.Base(member)))
		pins[member] = hex.EncodeToString(sum[:])
	}
	return Kit{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(raw)), Members: pins}, raw
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
	// Without a bound, reading this would never end.
	if _, err := kit.Read(io.MultiReader(bytes.NewReader(raw), rand.Reader)); err == nil {
		t.Error("an endless kit was accepted")
	}

	const dtb = "Linux_for_Tegra/kernel/dtb/tegra210-p3448-0002-p3449-0000-b00.dtb"
	kit.Members[dtb] = strings.Repeat("0", 64)
	if _, err := kit.Read(bytes.NewReader(raw)); err == nil || !strings.Contains(err.Error(), dtb) {
		t.Errorf("want an error naming the member that misses its pin, got %v", err)
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

func TestCacheRoundTripsTheMembers(t *testing.T) {
	kit, raw := fixtureKit(t, "kit.tbz2")
	in, err := kit.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()

	if err := kit.Cache(root, in); err != nil {
		t.Fatal(err)
	}
	got, err := kit.Cached(root)

	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Error("the cached members differ from the kit's")
	}
	if files, _ := os.ReadDir(filepath.Join(root, kit.SHA256)); len(files) != len(in.members()) {
		t.Errorf("cached %d files for %d members", len(files), len(in.members()))
	}
}

// A write cut short leaves either the temporary directory or, from a writer
// that wrote in place, a copy with members missing. Neither is a hit, and
// caching again repairs it.
func TestAnInterruptedCacheWriteIsNotAHit(t *testing.T) {
	kit, raw := fixtureKit(t, "kit.tbz2")
	in, err := kit.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".partial-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, kit.SHA256), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, kit.SHA256, "cboot.bin"), in.CBoot, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := kit.Cached(root); err == nil {
		t.Fatal("a copy with members missing was a hit")
	}
	if err := kit.Cache(root, in); err != nil {
		t.Fatal(err)
	}
	if _, err := kit.Cached(root); err != nil {
		t.Errorf("caching again didn't repair it: %v", err)
	}
}

func TestPruneKeepsOnlyThisKitsCopy(t *testing.T) {
	kit := Kit{SHA256: "current"}
	root := t.TempDir()
	for _, d := range []string{"current", "superseded", ".partial-7"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	kit.Prune(root)

	entries, _ := os.ReadDir(root)
	if len(entries) != 1 || entries[0].Name() != "current" {
		t.Errorf("left %v, want only the current kit's copy", entries)
	}
}
