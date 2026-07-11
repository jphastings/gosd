package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
)

// isolateUserCacheDir redirects os.UserCacheDir() (HOME on macOS,
// XDG_CACHE_HOME/HOME on Linux) into a throwaway directory, so a test
// exercising kernelFirmwareCacheDir() never touches - or depends on the
// contents of - the real user cache. gosd build also shells out to `go
// build` to cross-compile the app and gosd-init (internal/build.archEnv
// passes the test process's own os.Environ() straight through), so GOPATH/
// GOMODCACHE/GOCACHE are captured from the real environment first and
// re-exported explicitly - otherwise that subprocess would re-derive them
// from the fake HOME and re-populate a module cache under a t.TempDir(),
// which then fails to clean up (the module cache's files are read-only).
func isolateUserCacheDir(t *testing.T) {
	t.Helper()

	for _, key := range []string{"GOPATH", "GOMODCACHE", "GOCACHE"} {
		out, err := exec.Command("go", "env", key).Output()
		if err != nil {
			t.Fatalf("go env %s: %v", key, err)
		}
		t.Setenv(key, strings.TrimSpace(string(out)))
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmpHome, ".cache"))
}

// TestBuildPlacesGosdKernelTomlFirmwareUnderLibFirmware is the worked-example
// fixture test for gosd-hkp7's [[firmware]] flow: a gosd-kernel.toml with one
// firmware entry, passed to `gosd build --kernel-config`, ends up fetched
// (via internal/fetch, sha256-verified, from a local httptest.Server so no
// real network is touched) and embedded in the built image's initramfs at
// /lib/firmware/<dest>, alongside the board's own firmware.
func TestBuildPlacesGosdKernelTomlFirmwareUnderLibFirmware(t *testing.T) {
	isolateUserCacheDir(t)

	firmwareContent := []byte("fake-usb-dvb-firmware-blob\n")
	sum := sha256.Sum256(firmwareContent)
	sha := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(firmwareContent)
	}))
	defer srv.Close()

	kernelCfgPath := filepath.Join(t.TempDir(), "gosd-kernel.toml")
	kernelCfgContents := fmt.Sprintf(`
[[firmware]]
url = %q
sha256 = %q
dest = "vendor/usb-dvb.fw"
`, srv.URL, sha)
	if err := os.WriteFile(kernelCfgPath, []byte(kernelCfgContents), 0o644); err != nil {
		t.Fatalf("writing gosd-kernel.toml fixture: %v", err)
	}

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--kernel-config", kernelCfgPath,
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --kernel-config failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	records := decodeInitramfs(t, initramfsBytes)

	if !hasRecord(records, "lib/firmware/vendor/usb-dvb.fw") {
		t.Fatalf("initramfs is missing lib/firmware/vendor/usb-dvb.fw; got entries %v", recordNames(records))
	}
	got := recordContent(t, records, "lib/firmware/vendor/usb-dvb.fw")
	if string(got) != string(firmwareContent) {
		t.Errorf("lib/firmware/vendor/usb-dvb.fw content = %q, want %q", got, firmwareContent)
	}

	// The board's own WiFi firmware must still be present alongside the
	// developer-declared entry, not replaced by it.
	if !hasRecord(records, "lib/firmware/brcm/brcmfmac43436-sdio.bin") {
		t.Errorf("initramfs is missing the board's own firmware; got entries %v", recordNames(records))
	}
}

// TestBuildWithNoKernelConfigTouchesNoFirmwareNetwork confirms the common
// case - no gosd-kernel.toml, no --kernel-config - never attempts a firmware
// fetch, so gosd build's no-network --artifacts-dir guarantee holds even
// though the kernel-firmware code path now exists.
func TestBuildWithNoKernelConfigTouchesNoFirmwareNetwork(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s", r.URL)
		return nil, fmt.Errorf("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}
}

// TestBuildKernelConfigMissingExplicitPathErrorsActionably confirms `gosd
// build --kernel-config <missing file>` fails fast naming the path, the same
// as build-kernel's --config does, rather than silently building with no
// overlay.
func TestBuildKernelConfigMissingExplicitPathErrorsActionably(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--kernel-config", missing,
		"-o", filepath.Join(t.TempDir(), "hello-pi-zero-2w.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --kernel-config <missing file> succeeded, want an error")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error = %q, want it to name %q", err.Error(), missing)
	}
}
