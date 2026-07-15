package extbuild_test

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/extbuild"
)

// writeStaticELFFixture hand-crafts a minimal, valid, statically linked ELF
// binary (no program headers at all, so no PT_INTERP can be present) whose
// class/machine match arch - a real cross-compiled binary is unnecessary for
// exercising extbuild's own orchestration/cache/verification wiring, and
// avoids these tests needing a real Go cross-compile step.
func writeStaticELFFixture(path string, arch boards.Arch) error {
	class, machine, err := elfExpectationsFor(arch)
	if err != nil {
		return err
	}
	return writeELFFixture(path, class, machine, false)
}

// writeDynamicELFFixture hand-crafts a minimal ELF binary carrying a
// PT_INTERP program header (the hallmark of a dynamically linked binary),
// mirroring the fixture cmd/gosd's --with-external integration test uses.
func writeDynamicELFFixture(path string) error {
	return writeELFFixture(path, elf.ELFCLASS64, elf.EM_AARCH64, true)
}

func elfExpectationsFor(arch boards.Arch) (elf.Class, elf.Machine, error) {
	switch arch.GOARCH {
	case "arm64":
		return elf.ELFCLASS64, elf.EM_AARCH64, nil
	case "arm":
		return elf.ELFCLASS32, elf.EM_ARM, nil
	default:
		return 0, 0, fmt.Errorf("unsupported test fixture arch: %s", arch.Key())
	}
}

func writeELFFixture(path string, class elf.Class, machine elf.Machine, withInterp bool) error {
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
		if err := binary.Write(&buf, binary.LittleEndian, hdr); err != nil {
			return err
		}
		if withInterp {
			prog := elf.Prog64{
				Type: uint32(elf.PT_INTERP), Flags: uint32(elf.PF_R),
				Off: uint64(ehsize) + uint64(phentsize), Filesz: uint64(len(interp)), Memsz: uint64(len(interp)), Align: 1,
			}
			if err := binary.Write(&buf, binary.LittleEndian, prog); err != nil {
				return err
			}
			buf.WriteString(interp)
		}
	case elf.ELFCLASS32:
		ehsize := uint16(binary.Size(elf.Header32{}))
		phentsize := uint16(binary.Size(elf.Prog32{}))
		hdr := elf.Header32{
			Ident: ident, Type: uint16(elf.ET_EXEC), Machine: uint16(machine), Version: uint32(elf.EV_CURRENT),
			Entry: 0x8000, Phoff: uint32(ehsize), Ehsize: ehsize, Phentsize: phentsize, Phnum: phnum,
		}
		if err := binary.Write(&buf, binary.LittleEndian, hdr); err != nil {
			return err
		}
		if withInterp {
			prog := elf.Prog32{
				Type: uint32(elf.PT_INTERP), Off: uint32(ehsize) + uint32(phentsize),
				Filesz: uint32(len(interp)), Memsz: uint32(len(interp)), Flags: uint32(elf.PF_R), Align: 1,
			}
			if err := binary.Write(&buf, binary.LittleEndian, prog); err != nil {
				return err
			}
			buf.WriteString(interp)
		}
	}

	return os.WriteFile(path, buf.Bytes(), 0o755)
}

func TestOutput_SourceJSONContainsEveryProvenanceEntry(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	outDir := t.TempDir()

	if _, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt, CacheDir: t.TempDir(), OutputDir: outDir}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "source.json"))
	if err != nil {
		t.Fatalf("reading source.json: %v", err)
	}
	var got []extbuild.Source
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing source.json: %v", err)
	}
	if len(got) != 1 || got[0] != spec.Sources[0] {
		t.Errorf("source.json = %+v, want %+v", got, spec.Sources)
	}
}

func TestOutput_SourceJSONWrittenEvenWithNoSources(t *testing.T) {
	spec := testSpec()
	spec.Sources = nil
	rt := newSucceedingRunner(spec)
	outDir := t.TempDir()

	if _, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt, CacheDir: t.TempDir(), OutputDir: outDir}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "source.json"))
	if err != nil {
		t.Fatalf("reading source.json: %v", err)
	}
	var got []extbuild.Source
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing source.json: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("source.json = %v, want an empty array", got)
	}
}

func TestOutput_NoOutputDirStillReturnsCacheOutputPath(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)

	result, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := os.Stat(result.OutputPath); err != nil {
		t.Errorf("Result.OutputPath %s does not exist: %v", result.OutputPath, err)
	}
	if filepath.Dir(result.OutputPath) != result.CacheDir {
		t.Errorf("OutputPath = %s, want it inside CacheDir %s", result.OutputPath, result.CacheDir)
	}
}
