package initramfs

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/u-root/u-root/pkg/cpio"
)

func testSpec(t *testing.T) Spec {
	t.Helper()

	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("reading testdata/%s: %v", name, err)
		}
		return b
	}

	return Spec{
		Files: []File{
			{Path: "/init", Content: bytes.NewReader(read("init.bin")), Mode: 0o755},
			{Path: "/app", Content: bytes.NewReader(read("app.bin")), Mode: 0o755},
			{Path: "/lib/firmware/wifi.bin", Content: bytes.NewReader(read("wifi.bin")), Mode: 0o644},
			{Path: "/etc/gosd/config.json", Content: bytes.NewReader(read("config.json")), Mode: 0o644},
		},
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	if err := Build(&buf1, testSpec(t)); err != nil {
		t.Fatalf("Build (1): %v", err)
	}
	if err := Build(&buf2, testSpec(t)); err != nil {
		t.Fatalf("Build (2): %v", err)
	}

	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Fatal("Build produced different output for an identical Spec; want byte-identical rebuilds")
	}
}

func TestBuildEntries(t *testing.T) {
	var buf bytes.Buffer
	if err := Build(&buf, testSpec(t)); err != nil {
		t.Fatalf("Build: %v", err)
	}

	records := decodeArchive(t, buf.Bytes())

	wantOrder := []string{
		"app",
		"etc",
		"etc/gosd",
		"etc/gosd/config.json",
		"init",
		"lib",
		"lib/firmware",
		"lib/firmware/wifi.bin",
	}
	if got := recordNames(records); !equalStrings(got, wantOrder) {
		t.Fatalf("archive entries = %v, want exactly %v (sorted, explicit parent dirs)", got, wantOrder)
	}

	wantModes := map[string]uint64{
		"app":                   cpio.S_IFREG | 0o755,
		"etc":                   cpio.S_IFDIR | 0o755,
		"etc/gosd":              cpio.S_IFDIR | 0o755,
		"etc/gosd/config.json":  cpio.S_IFREG | 0o644,
		"init":                  cpio.S_IFREG | 0o755,
		"lib":                   cpio.S_IFDIR | 0o755,
		"lib/firmware":          cpio.S_IFDIR | 0o755,
		"lib/firmware/wifi.bin": cpio.S_IFREG | 0o644,
	}

	for _, r := range records {
		if r.Info.MTime != 0 {
			t.Errorf("record %q MTime = %d, want 0 (epoch)", r.Info.Name, r.Info.MTime)
		}
		if want := wantModes[r.Info.Name]; r.Info.Mode != want {
			t.Errorf("record %q Mode = %#o, want %#o", r.Info.Name, r.Info.Mode, want)
		}
	}

	assertContent(t, records, "init", "init.bin")
	assertContent(t, records, "app", "app.bin")
	assertContent(t, records, "lib/firmware/wifi.bin", "wifi.bin")
	assertContent(t, records, "etc/gosd/config.json", "config.json")
}

func TestBuildRejectsDuplicatePaths(t *testing.T) {
	spec := Spec{Files: []File{
		{Path: "/init", Content: bytes.NewReader(nil), Mode: 0o755},
		{Path: "/init", Content: bytes.NewReader(nil), Mode: 0o755},
	}}

	if err := Build(io.Discard, spec); err == nil {
		t.Fatal("Build with duplicate paths succeeded, want an error")
	}
}

func TestBuildRejectsFileDirectoryCollision(t *testing.T) {
	spec := Spec{Files: []File{
		{Path: "/lib/firmware", Content: bytes.NewReader(nil), Mode: 0o644},
		{Path: "/lib/firmware/wifi.bin", Content: bytes.NewReader(nil), Mode: 0o644},
	}}

	if err := Build(io.Discard, spec); err == nil {
		t.Fatal("Build with a path used as both a file and a directory succeeded, want an error")
	}
}

func assertContent(t *testing.T, records []cpio.Record, name, fixture string) {
	t.Helper()

	want, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", fixture, err)
	}

	for _, r := range records {
		if r.Info.Name != name {
			continue
		}
		got := make([]byte, r.Info.FileSize)
		if _, err := r.ReaderAt.ReadAt(got, 0); err != nil && err != io.EOF {
			t.Fatalf("reading record %q content: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("record %q content = %q, want %q", name, got, want)
		}
		return
	}
	t.Fatalf("no record named %q found in archive", name)
}

func recordNames(records []cpio.Record) []string {
	out := make([]string, len(records))
	for i, r := range records {
		out[i] = r.Info.Name
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// decodeArchive decompresses and parses compressed with the same cpio
// package Build uses, returning every record except the trailer.
func decodeArchive(t *testing.T, compressed []byte) []cpio.Record {
	t.Helper()

	zr, err := zstd.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("creating zstd reader: %v", err)
	}
	defer zr.Close()

	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompressing archive: %v", err)
	}

	records, err := cpio.ReadAllRecords(cpio.Newc.Reader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("reading cpio records: %v", err)
	}
	return records
}
