package inject_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/inject"
)

func TestRenderIsDeterministic(t *testing.T) {
	p := inject.Placeholder{Path: "backupist.yaml", SizeBytes: 4096}

	got1, err := inject.Render(p)
	if err != nil {
		t.Fatalf("Render #1: %v", err)
	}
	got2, err := inject.Render(p)
	if err != nil {
		t.Fatalf("Render #2: %v", err)
	}
	if !bytes.Equal(got1, got2) {
		t.Error("two Render() calls with the same Placeholder produced different bytes")
	}
}

// minRenderedSize discovers the smallest SizeBytes Render will accept for
// path by parsing it back out of Validate's own "too small" error, which
// states the exact minimum - this keeps the test from hardcoding a number
// that would silently drift out of sync with Render's header format.
func minRenderedSize(t *testing.T, path string) int64 {
	t.Helper()

	_, err := inject.Render(inject.Placeholder{Path: path, SizeBytes: 1})
	if err == nil {
		t.Fatalf("Render(%q, SizeBytes: 1) unexpectedly succeeded", path)
	}

	const marker = "minimum size for this path is "
	msg := err.Error()
	i := strings.Index(msg, marker)
	if i < 0 {
		t.Fatalf("error %q doesn't contain the expected %q marker", msg, marker)
	}
	rest := msg[i+len(marker):]
	end := strings.Index(rest, " bytes")
	if end < 0 {
		t.Fatalf("error %q has no ' bytes' suffix after the minimum", msg)
	}
	min, err := strconv.ParseInt(rest[:end], 10, 64)
	if err != nil {
		t.Fatalf("parsing minimum %q: %v", rest[:end], err)
	}
	return min
}

func TestRenderSizesAndContentsAcrossSeveralSizes(t *testing.T) {
	const path = "backupist.yaml"
	min := minRenderedSize(t, path)

	for _, size := range []int64{min, min + 1, min + 1000, 4096, 32768} {
		t.Run(strconv.FormatInt(size, 10), func(t *testing.T) {
			got, err := inject.Render(inject.Placeholder{Path: path, SizeBytes: size})
			if err != nil {
				t.Fatalf("Render(size=%d): %v", size, err)
			}
			if int64(len(got)) != size {
				t.Fatalf("Render(size=%d) produced %d bytes", size, len(got))
			}
			if got[len(got)-1] != '\n' {
				t.Errorf("Render(size=%d) does not end with '\\n'", size)
			}
			if wantPrefix := "# GOSD-PLACEHOLDER v1 path=" + path; !bytes.HasPrefix(got, []byte(wantPrefix)) {
				t.Errorf("Render(size=%d) does not start with %q", size, wantPrefix)
			}

			var doc any
			if err := yaml.Unmarshal(got, &doc); err != nil {
				t.Errorf("Render(size=%d) output is not valid YAML: %v", size, err)
			}
		})
	}
}

func TestRenderRejectsSizeBelowMinimum(t *testing.T) {
	min := minRenderedSize(t, "backupist.yaml")

	_, err := inject.Render(inject.Placeholder{Path: "backupist.yaml", SizeBytes: min - 1})
	if err == nil {
		t.Fatal("Render() with a too-small size succeeded, want an error")
	}
	if !strings.Contains(err.Error(), strconv.FormatInt(min, 10)) {
		t.Errorf("error = %q, want it to state the minimum (%d)", err, min)
	}
}

func TestValidateRejectsInvalidPaths(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
	}{
		{"empty", ""},
		{"leading slash", "/backupist.yaml"},
		{"dot segment", "./backupist.yaml"},
		{"dot-dot segment", "../backupist.yaml"},
		{"dot-dot in the middle", "a/../b.yaml"},
		{"spaces", "back up ist.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := inject.Placeholder{Path: tc.path, SizeBytes: 1 << 20}
			if err := p.Validate(); err == nil {
				t.Errorf("Validate() with path %q succeeded, want an error", tc.path)
			}
		})
	}
}

func TestValidateRejectsSizeAboveFAT32Limit(t *testing.T) {
	p := inject.Placeholder{Path: "backupist.yaml", SizeBytes: 4*1024*1024*1024 + 1}
	if err := p.Validate(); err == nil {
		t.Fatal("Validate() with a size above 4GiB-1 succeeded, want an error")
	}
}

func TestManifestPathReplacesExtension(t *testing.T) {
	got := inject.ManifestPath("/out/atbackup-pi-zero-2w.img")
	want := "/out/atbackup-pi-zero-2w.inject.json"
	if got != want {
		t.Errorf("ManifestPath() = %q, want %q", got, want)
	}
}

func TestWriteManifestWritesTheDocumentedSchema(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "app-pi-zero-2w.img")
	imgContent := []byte("pretend image bytes, enough to hash\n")
	if err := os.WriteFile(imgPath, imgContent, 0o644); err != nil {
		t.Fatalf("writing fake image: %v", err)
	}

	placeholders := []inject.Placeholder{
		{Path: "backupist.yaml", SizeBytes: 4096},
		{Path: "network-config", SizeBytes: 2048},
	}
	fileRanges := map[string][]image.ByteRange{
		"backupist.yaml": {{OffsetBytes: 17301504, LengthBytes: 4096}},
		"network-config": {{OffsetBytes: 17334272, LengthBytes: 1024}, {OffsetBytes: 17336320, LengthBytes: 1024}},
	}

	manifestPath, err := inject.WriteManifest(imgPath, inject.ManifestSpec{Board: "pi-zero-2w", Placeholders: placeholders, FileRanges: fileRanges})
	if err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if want := inject.ManifestPath(imgPath); manifestPath != want {
		t.Errorf("WriteManifest returned %q, want %q", manifestPath, want)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}
	if data[len(data)-1] != '\n' {
		t.Error("manifest file does not end with a trailing newline")
	}

	var m inject.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}

	if m.GosdInject != 1 {
		t.Errorf("gosd_inject = %d, want 1", m.GosdInject)
	}
	if m.Board != "pi-zero-2w" {
		t.Errorf("board = %q, want pi-zero-2w", m.Board)
	}
	if m.Image.Filename != "app-pi-zero-2w.img" {
		t.Errorf("image.filename = %q, want app-pi-zero-2w.img", m.Image.Filename)
	}
	if m.Image.Size != int64(len(imgContent)) {
		t.Errorf("image.size = %d, want %d", m.Image.Size, len(imgContent))
	}
	wantImageSum := sha256.Sum256(imgContent)
	if m.Image.SHA256 != hex.EncodeToString(wantImageSum[:]) {
		t.Errorf("image.sha256 = %q, want %q", m.Image.SHA256, hex.EncodeToString(wantImageSum[:]))
	}

	if len(m.Placeholders) != 2 {
		t.Fatalf("len(placeholders) = %d, want 2", len(m.Placeholders))
	}
	for i, p := range placeholders {
		info := m.Placeholders[i]
		if info.Path != p.Path {
			t.Errorf("placeholders[%d].path = %q, want %q", i, info.Path, p.Path)
		}
		if info.Size != p.SizeBytes {
			t.Errorf("placeholders[%d].size = %d, want %d", i, info.Size, p.SizeBytes)
		}

		rendered, err := inject.Render(p)
		if err != nil {
			t.Fatalf("re-rendering %q: %v", p.Path, err)
		}
		wantSum := sha256.Sum256(rendered)
		if info.SHA256 != hex.EncodeToString(wantSum[:]) {
			t.Errorf("placeholders[%d].sha256 = %q, want %q (sha256 of the rendered content)", i, info.SHA256, hex.EncodeToString(wantSum[:]))
		}

		wantRanges := fileRanges[p.Path]
		if len(info.Ranges) != len(wantRanges) {
			t.Fatalf("placeholders[%d] has %d ranges, want %d", i, len(info.Ranges), len(wantRanges))
		}
		for j, r := range wantRanges {
			if info.Ranges[j].Offset != r.OffsetBytes || info.Ranges[j].Length != r.LengthBytes {
				t.Errorf("placeholders[%d].ranges[%d] = %+v, want {Offset:%d Length:%d}", i, j, info.Ranges[j], r.OffsetBytes, r.LengthBytes)
			}
		}
	}
}

func TestWriteManifestErrorsOnMissingFileRangesEntry(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "app-pi-zero-2w.img")
	if err := os.WriteFile(imgPath, []byte("bytes\n"), 0o644); err != nil {
		t.Fatalf("writing fake image: %v", err)
	}

	placeholders := []inject.Placeholder{{Path: "backupist.yaml", SizeBytes: 4096}}

	_, err := inject.WriteManifest(imgPath, inject.ManifestSpec{Board: "pi-zero-2w", Placeholders: placeholders, FileRanges: map[string][]image.ByteRange{}})
	if err == nil {
		t.Fatal("WriteManifest() with no fileRanges entry for the placeholder succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "backupist.yaml") {
		t.Errorf("error = %q, want it to name the placeholder path", err)
	}
}
