package staticelf_test

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/staticelf"
)

var arm64 = boards.Arch{GOARCH: "arm64"}
var armv6 = boards.Arch{GOARCH: "arm", GOARM: "6"}

// writeELF hand-crafts a minimal ELF binary at path with the given
// class/machine, optionally carrying a PT_INTERP program header (the
// hallmark of a dynamically linked binary). A real dynamically linked
// binary is impractical to reproduce portably in a test (it'd need a cross
// C toolchain), so this constructs the smallest file debug/elf will parse,
// mirroring the fixture cmd/gosd's --with-external integration test uses.
func writeELF(t *testing.T, path string, class elf.Class, machine elf.Machine, withInterp bool) {
	t.Helper()

	const interp = "/lib/ld-linux.so\x00"

	ident := [elf.EI_NIDENT]byte{}
	ident[0], ident[1], ident[2], ident[3] = 0x7f, 'E', 'L', 'F'
	ident[elf.EI_CLASS] = byte(class)
	ident[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	ident[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	ident[elf.EI_OSABI] = byte(elf.ELFOSABI_NONE)

	var buf bytes.Buffer
	var phnum uint16
	if withInterp {
		phnum = 1
	}

	switch class {
	case elf.ELFCLASS64:
		ehsize := uint16(binary.Size(elf.Header64{}))
		phentsize := uint16(binary.Size(elf.Prog64{}))
		hdr := elf.Header64{
			Ident: ident, Type: uint16(elf.ET_EXEC), Machine: uint16(machine), Version: uint32(elf.EV_CURRENT),
			Entry: 0x400000, Phoff: uint64(ehsize), Ehsize: ehsize, Phentsize: phentsize, Phnum: phnum,
		}
		mustWrite(t, &buf, hdr)
		if withInterp {
			prog := elf.Prog64{
				Type: uint32(elf.PT_INTERP), Flags: uint32(elf.PF_R),
				Off: uint64(ehsize) + uint64(phentsize), Filesz: uint64(len(interp)), Memsz: uint64(len(interp)), Align: 1,
			}
			mustWrite(t, &buf, prog)
			buf.WriteString(interp)
		}
	case elf.ELFCLASS32:
		ehsize := uint16(binary.Size(elf.Header32{}))
		phentsize := uint16(binary.Size(elf.Prog32{}))
		hdr := elf.Header32{
			Ident: ident, Type: uint16(elf.ET_EXEC), Machine: uint16(machine), Version: uint32(elf.EV_CURRENT),
			Entry: 0x8000, Phoff: uint32(ehsize), Ehsize: ehsize, Phentsize: phentsize, Phnum: phnum,
		}
		mustWrite(t, &buf, hdr)
		if withInterp {
			prog := elf.Prog32{
				Type: uint32(elf.PT_INTERP), Off: uint32(ehsize) + uint32(phentsize),
				Filesz: uint32(len(interp)), Memsz: uint32(len(interp)), Flags: uint32(elf.PF_R), Align: 1,
			}
			mustWrite(t, &buf, prog)
			buf.WriteString(interp)
		}
	default:
		t.Fatalf("writeELF: unsupported class %v", class)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("writing ELF fixture %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, buf *bytes.Buffer, v any) {
	t.Helper()
	if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
		t.Fatalf("encoding ELF fixture field %T: %v", v, err)
	}
}

func openFixture(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening fixture %s: %v", path, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestVerifyAcceptsMatchingStaticBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "static-arm64")
	writeELF(t, path, elf.ELFCLASS64, elf.EM_AARCH64, false)

	if err := staticelf.Verify(openFixture(t, path), path, arm64); err != nil {
		t.Errorf("Verify(matching static arm64 binary) = %v, want nil", err)
	}
}

func TestVerifyAcceptsMatchingStaticBinaryForArmv6(t *testing.T) {
	path := filepath.Join(t.TempDir(), "static-armv6")
	writeELF(t, path, elf.ELFCLASS32, elf.EM_ARM, false)

	if err := staticelf.Verify(openFixture(t, path), path, armv6); err != nil {
		t.Errorf("Verify(matching static armv6 binary) = %v, want nil", err)
	}
}

func TestVerifyRejectsNonELFFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-binary")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	err := staticelf.Verify(openFixture(t, path), path, arm64)
	if err == nil {
		t.Fatal("Verify(non-ELF file) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "ELF") {
		t.Errorf("error = %q, want it to mention ELF", err.Error())
	}
	var notELF *staticelf.NotELFError
	if !errors.As(err, &notELF) {
		t.Errorf("error type = %T, want *staticelf.NotELFError", err)
	}
}

func TestVerifyRejectsArchMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wrong-arch")
	writeELF(t, path, elf.ELFCLASS64, elf.EM_AARCH64, false)

	err := staticelf.Verify(openFixture(t, path), path, armv6)
	if err == nil {
		t.Fatal("Verify(arm64 binary against armv6) succeeded, want an error")
	}
	mismatch, ok := err.(*staticelf.MismatchError)
	if !ok {
		t.Fatalf("error type = %T, want *staticelf.MismatchError", err)
	}
	if mismatch.GotClass != elf.ELFCLASS64 || mismatch.WantClass != elf.ELFCLASS32 {
		t.Errorf("MismatchError = %+v, want GotClass=64/WantClass=32", mismatch)
	}
}

func TestVerifyRejectsDynamicallyLinkedBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dynamic")
	writeELF(t, path, elf.ELFCLASS64, elf.EM_AARCH64, true)

	err := staticelf.Verify(openFixture(t, path), path, arm64)
	if err == nil {
		t.Fatal("Verify(dynamically linked binary) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "static") {
		t.Errorf("error = %q, want it to explain the binary must be static", err.Error())
	}
	if _, ok := err.(*staticelf.DynamicallyLinkedError); !ok {
		t.Errorf("error type = %T, want *staticelf.DynamicallyLinkedError", err)
	}
}

func TestExpectationsErrorsOnUnknownGOARCH(t *testing.T) {
	_, _, err := staticelf.Expectations(boards.Arch{GOARCH: "riscv64"})
	if err == nil {
		t.Fatal("Expectations(riscv64) succeeded, want an error (gosd doesn't validate that arch yet)")
	}
}

func TestGOARMSuffix(t *testing.T) {
	if got := staticelf.GOARMSuffix(arm64); got != "" {
		t.Errorf("GOARMSuffix(arm64) = %q, want empty", got)
	}
	if got := staticelf.GOARMSuffix(armv6); got != " GOARM=6" {
		t.Errorf("GOARMSuffix(armv6) = %q, want \" GOARM=6\"", got)
	}
}
