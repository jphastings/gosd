package main

import (
	"bytes"
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
)

// TestBuildAppliesPerBoardBuildTags is the full end-to-end acceptance test
// for gosd-1937: a real `gosd build` for two boards sharing an arch
// (pi-zero-2w and nanopi-zero2, both arm64) against testdata/boardtagfixture
// (a fallback main.go plus a gosd_pi_zero_2w- and a gosd_nanopi_zero2-gated
// variant, see that package) must compile each board's own tagged variant
// into /app - not the fallback, and not the other board's variant - while
// still sharing exactly one gosd-init compile pass (byte-identical /init
// across both images), preserving the per-arch dedupe gosd-2j6z established.
func TestBuildAppliesPerBoardBuildTags(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	outDir := t.TempDir()

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "./testdata/boardtagfixture",
		"--board", "pi-zero-2w",
		"--board", "nanopi-zero2",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", outDir,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	piApp := readAppBinary(t, filepath.Join(outDir, "boardtagfixture-pi-zero-2w.img"))
	nanopiApp := readAppBinary(t, filepath.Join(outDir, "boardtagfixture-nanopi-zero2.img"))

	for _, tc := range []struct {
		board string
		app   []byte
		want  string
	}{
		{"pi-zero-2w", piApp, "boardtagfixture-marker:pi-zero-2w"},
		{"nanopi-zero2", nanopiApp, "boardtagfixture-marker:nanopi-zero2"},
	} {
		if !bytes.Contains(tc.app, []byte(tc.want)) {
			t.Errorf("%s's /app does not contain %q; the board-specific gosd_%s-tagged file was not selected", tc.board, tc.want, tc.board)
		}
		if bytes.Contains(tc.app, []byte("boardtagfixture-marker:default")) {
			t.Errorf("%s's /app contains the fallback marker; the board tag should have excluded main.go's default build", tc.board)
		}
	}
	if bytes.Contains(piApp, []byte("boardtagfixture-marker:nanopi-zero2")) {
		t.Error("pi-zero-2w's /app contains nanopi-zero2's marker; each board must get its own app compile")
	}
	if bytes.Contains(nanopiApp, []byte("boardtagfixture-marker:pi-zero-2w")) {
		t.Error("nanopi-zero2's /app contains pi-zero-2w's marker; each board must get its own app compile")
	}

	piInit := readInitBinary(t, filepath.Join(outDir, "boardtagfixture-pi-zero-2w.img"))
	nanopiInit := readInitBinary(t, filepath.Join(outDir, "boardtagfixture-nanopi-zero2.img"))
	if !bytes.Equal(piInit, nanopiInit) {
		t.Error("gosd-init differs between pi-zero-2w and nanopi-zero2 (both arm64); per-board app tagging must not affect gosd-init's per-arch dedupe")
	}
}

// readAppBinary opens imgPath and returns the initramfs's /app record's raw
// content.
func readAppBinary(t *testing.T, imgPath string) []byte {
	t.Helper()
	return readInitramfsRecord(t, imgPath, "app")
}

// readInitBinary opens imgPath and returns the initramfs's /init record's
// raw content.
func readInitBinary(t *testing.T, imgPath string) []byte {
	t.Helper()
	return readInitramfsRecord(t, imgPath, "init")
}

func readInitramfsRecord(t *testing.T, imgPath, name string) []byte {
	t.Helper()

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image %s failed: %v", imgPath, err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) for %s failed: %v", imgPath, err)
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst from %s: %v", imgPath, err)
	}

	records := decodeInitramfs(t, initramfsBytes)
	return recordContent(t, records, name)
}
