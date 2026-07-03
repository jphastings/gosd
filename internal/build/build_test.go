package build

import (
	"debug/elf"
	"path/filepath"
	"testing"
)

func TestCrossCompileProducesStaticARM64Binary(t *testing.T) {
	out := filepath.Join(t.TempDir(), "hello")

	if err := CrossCompile("./testdata/hello", out); err != nil {
		t.Fatalf("CrossCompile: %v", err)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("output is not a valid ELF binary: %v", err)
	}
	defer f.Close()

	if f.Class != elf.ELFCLASS64 {
		t.Errorf("Class = %v, want %v (64-bit)", f.Class, elf.ELFCLASS64)
	}
	if f.Machine != elf.EM_AARCH64 {
		t.Errorf("Machine = %v, want %v (arm64)", f.Machine, elf.EM_AARCH64)
	}

	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			t.Errorf("binary has a PT_INTERP segment, meaning it needs a dynamic linker; want a statically linked binary")
		}
	}
}

func TestCrossCompileRejectsNonMainPackage(t *testing.T) {
	out := filepath.Join(t.TempDir(), "notmain")

	err := CrossCompile("./testdata/notmain", out)
	if err == nil {
		t.Fatal("CrossCompile succeeded on a non-main package, want an error")
	}
}

func TestCrossCompileSurfacesBuildFailure(t *testing.T) {
	err := CrossCompile("./testdata/doesnotexist", filepath.Join(t.TempDir(), "out"))
	if err == nil {
		t.Fatal("CrossCompile succeeded on a missing package, want an error")
	}
}
