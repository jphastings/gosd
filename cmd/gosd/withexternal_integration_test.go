package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/u-root/u-root/pkg/cpio"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/build"
)

// crossCompileFixture cross-compiles internal/build/testdata/hello - a
// trivial `package main` - for arch with CGO_ENABLED=0, producing a
// genuinely static ELF binary shaped exactly like a real --with-external
// input. This is preferred over a synthetic ELF for the happy-path tests:
// it's real proof gosd build accepts what a real static Go cross-compile
// produces.
func crossCompileFixture(t *testing.T, arch boards.Arch) string {
	t.Helper()

	out := filepath.Join(t.TempDir(), "fixture-"+arch.Key())
	if err := build.CrossCompile("../../internal/build/testdata/hello", out, "", arch); err != nil {
		t.Fatalf("cross-compiling fixture binary for %s: %v", arch.Key(), err)
	}
	return out
}

// writeDynamicELF writes a minimal, hand-crafted ELF binary at path whose
// class/machine match the given values, but which carries a PT_INTERP
// program header (the hallmark of a dynamically linked binary) - real
// dynamically linked binaries are impractical to reproduce portably in a
// test (they'd need a cross C toolchain), so this constructs the smallest
// file debug/elf will parse as "dynamically linked" instead.
func writeDynamicELF(t *testing.T, path string, class elf.Class, machine elf.Machine) {
	t.Helper()

	const interp = "/lib/ld-linux.so\x00"

	ident := [elf.EI_NIDENT]byte{}
	ident[0], ident[1], ident[2], ident[3] = 0x7f, 'E', 'L', 'F'
	ident[elf.EI_CLASS] = byte(class)
	ident[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	ident[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	ident[elf.EI_OSABI] = byte(elf.ELFOSABI_NONE)

	var buf bytes.Buffer
	switch class {
	case elf.ELFCLASS64:
		ehsize := uint16(binary.Size(elf.Header64{}))
		phentsize := uint16(binary.Size(elf.Prog64{}))
		hdr := elf.Header64{
			Ident:     ident,
			Type:      uint16(elf.ET_EXEC),
			Machine:   uint16(machine),
			Version:   uint32(elf.EV_CURRENT),
			Entry:     0x400000,
			Phoff:     uint64(ehsize),
			Ehsize:    ehsize,
			Phentsize: phentsize,
			Phnum:     1,
		}
		prog := elf.Prog64{
			Type:   uint32(elf.PT_INTERP),
			Flags:  uint32(elf.PF_R),
			Off:    uint64(ehsize) + uint64(phentsize),
			Filesz: uint64(len(interp)),
			Memsz:  uint64(len(interp)),
			Align:  1,
		}
		mustWrite(t, &buf, hdr)
		mustWrite(t, &buf, prog)
		buf.WriteString(interp)
	case elf.ELFCLASS32:
		ehsize := uint16(binary.Size(elf.Header32{}))
		phentsize := uint16(binary.Size(elf.Prog32{}))
		hdr := elf.Header32{
			Ident:     ident,
			Type:      uint16(elf.ET_EXEC),
			Machine:   uint16(machine),
			Version:   uint32(elf.EV_CURRENT),
			Entry:     0x8000,
			Phoff:     uint32(ehsize),
			Ehsize:    ehsize,
			Phentsize: phentsize,
			Phnum:     1,
		}
		prog := elf.Prog32{
			Type:   uint32(elf.PT_INTERP),
			Off:    uint32(ehsize) + uint32(phentsize),
			Filesz: uint32(len(interp)),
			Memsz:  uint32(len(interp)),
			Flags:  uint32(elf.PF_R),
			Align:  1,
		}
		mustWrite(t, &buf, hdr)
		mustWrite(t, &buf, prog)
		buf.WriteString(interp)
	default:
		t.Fatalf("writeDynamicELF: unsupported class %v", class)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("writing dynamic ELF fixture %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, buf *bytes.Buffer, v any) {
	t.Helper()
	if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
		t.Fatalf("encoding ELF fixture field %T: %v", v, err)
	}
}

// noNetworkTransport is a shared network-tripwire RoundTripper: every test
// in this file uses --artifacts-dir, so a real network request means
// something regressed.
func noNetworkTransport(t *testing.T) {
	t.Helper()
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })
}

// TestBuildWithExternalBundlesStaticBinaryAtDefaultDest is the acceptance
// test for gosd-ig4h: a real static, cross-compiled binary passed via
// --with-external with no explicit dest lands in the initramfs at
// /bin/<basename>, mode 0755.
func TestBuildWithExternalBundlesStaticBinaryAtDefaultDest(t *testing.T) {
	noNetworkTransport(t)

	b, _ := boards.Find("pi-zero-2w")
	fixture := crossCompileFixture(t, b.Arch())

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--with-external", fixture,
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --with-external failed: %v", err)
	}

	records := readImageInitramfs(t, imgPath)
	wantName := "bin/" + filepath.Base(fixture)
	rec, ok := findRecord(records, wantName)
	if !ok {
		t.Fatalf("initramfs is missing %q; got entries %v", wantName, recordNames(records))
	}
	if mode := rec.Mode & 0o777; mode != 0o755 {
		t.Errorf("%s mode = %#o, want 0755", wantName, mode)
	}

	got := recordContent(t, records, wantName)
	want, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("reading fixture back: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s content does not match the fixture binary's bytes", wantName)
	}
}

// TestBuildWithExternalBundlesStaticBinaryAtExplicitDest covers the
// <path>:<dest> form, including a dest that isn't under /bin.
func TestBuildWithExternalBundlesStaticBinaryAtExplicitDest(t *testing.T) {
	noNetworkTransport(t)

	b, _ := boards.Find("pi-zero-2w")
	fixture := crossCompileFixture(t, b.Arch())

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--with-external", fixture + ":/usr/local/bin/companion",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --with-external failed: %v", err)
	}

	records := readImageInitramfs(t, imgPath)
	rec, ok := findRecord(records, "usr/local/bin/companion")
	if !ok {
		t.Fatalf("initramfs is missing usr/local/bin/companion; got entries %v", recordNames(records))
	}
	if mode := rec.Mode & 0o777; mode != 0o755 {
		t.Errorf("usr/local/bin/companion mode = %#o, want 0755", mode)
	}
}

// TestBuildWithExternalWorksAcrossBoardsSharingAnArch confirms a single
// --with-external flag is validated and embedded independently for every
// selected board that shares its binary's arch (arm64: pi-zero-2w,
// radxa-zero-3e, and nanopi-zero2), each getting its own copy in its own
// initramfs.
func TestBuildWithExternalWorksAcrossBoardsSharingAnArch(t *testing.T) {
	noNetworkTransport(t)

	b, _ := boards.Find("pi-zero-2w")
	fixture := crossCompileFixture(t, b.Arch())

	outDir := t.TempDir()
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--board", "radxa-zero-3e",
		"--board", "nanopi-zero2",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--with-external", fixture,
		"-o", outDir,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --with-external (multi-board) failed: %v", err)
	}

	for _, img := range []string{"hello-pi-zero-2w.img", "hello-radxa-zero-3e.img", "hello-nanopi-zero2.img"} {
		records := readImageInitramfs(t, filepath.Join(outDir, img))
		wantName := "bin/" + filepath.Base(fixture)
		if !hasRecord(records, wantName) {
			t.Errorf("%s: initramfs is missing %q", img, wantName)
		}
	}
}

// TestBuildWithExternalRejectsArchMismatch confirms an arm64 binary handed
// to an armv6 board (pi-zero-w) fails fast with an actionable error naming
// the board, rather than silently shipping an unbootable companion binary.
func TestBuildWithExternalRejectsArchMismatch(t *testing.T) {
	arm64, _ := boards.Find("pi-zero-2w")
	fixture := crossCompileFixture(t, arm64.Arch())

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--with-external", fixture,
		"-o", filepath.Join(t.TempDir(), "hello-pi-zero-w.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --with-external with an arm64 binary on --board pi-zero-w succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "pi-zero-w") {
		t.Errorf("error = %q, want it to name the board pi-zero-w", err.Error())
	}
}

// TestBuildWithExternalRejectsDynamicallyLinkedBinary confirms a binary with
// a PT_INTERP program header (i.e. one that needs a dynamic loader) is
// rejected with an actionable, static-only error, since the initramfs has
// no ld.so or library layout to resolve it against.
func TestBuildWithExternalRejectsDynamicallyLinkedBinary(t *testing.T) {
	dynPath := filepath.Join(t.TempDir(), "dynamic-mpv")
	writeDynamicELF(t, dynPath, elf.ELFCLASS64, elf.EM_AARCH64)

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--with-external", dynPath,
		"-o", filepath.Join(t.TempDir(), "hello-pi-zero-2w.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --with-external with a dynamically linked binary succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "static") {
		t.Errorf("error = %q, want it to explain the binary must be static", err.Error())
	}
}

// TestBuildWithExternalRejectsNonELFFile confirms a plain non-ELF file
// (e.g. a shell script) is rejected with an actionable error rather than a
// bare parse-error chain.
func TestBuildWithExternalRejectsNonELFFile(t *testing.T) {
	scriptPath := filepath.Join(t.TempDir(), "not-a-binary")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("writing fixture script: %v", err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--with-external", scriptPath,
		"-o", filepath.Join(t.TempDir(), "hello-pi-zero-2w.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --with-external with a non-ELF file succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "ELF") {
		t.Errorf("error = %q, want it to mention ELF", err.Error())
	}
}

// TestBuildWithExternalRejectsCollisionWithReservedDest confirms the
// build-time validation (not just an initramfs-level duplicate-path error)
// catches a --with-external dest colliding with a gosd-reserved path, with
// an actionable message.
func TestBuildWithExternalRejectsCollisionWithReservedDest(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--with-external", "./testdata/fake-artifacts:/app",
		"-o", filepath.Join(t.TempDir(), "hello-pi-zero-2w.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --with-external with dest /app succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "/app") {
		t.Errorf("error = %q, want it to mention the colliding dest /app", err.Error())
	}
}

func readImageInitramfs(t *testing.T, imgPath string) []cpio.Record {
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

	return decodeInitramfs(t, initramfsBytes)
}
