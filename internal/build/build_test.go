package build

import (
	"debug/elf"
	"path/filepath"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
)

var arm64 = boards.Arch{GOARCH: "arm64"}

func TestCrossCompileProducesStaticARM64Binary(t *testing.T) {
	out := filepath.Join(t.TempDir(), "hello")

	if err := CrossCompile("./testdata/hello", out, "", arm64); err != nil {
		t.Fatalf("CrossCompile: %v", err)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("output is not a valid ELF binary: %v", err)
	}
	defer func() { _ = f.Close() }()

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

// TestCrossCompileProducesStaticARMv6Binary is the keystone test for
// gosd-2j6z: a board whose Arch is GOARCH=arm/GOARM=6 (the upcoming
// pi-zero-w, bean gosd-et0q) must get a real static 32-bit ARM binary out of
// CrossCompile, not just an arm64 one with different env vars ignored.
func TestCrossCompileProducesStaticARMv6Binary(t *testing.T) {
	out := filepath.Join(t.TempDir(), "hello")

	if err := CrossCompile("./testdata/hello", out, "", boards.Arch{GOARCH: "arm", GOARM: "6"}); err != nil {
		t.Fatalf("CrossCompile: %v", err)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("output is not a valid ELF binary: %v", err)
	}
	defer func() { _ = f.Close() }()

	if f.Class != elf.ELFCLASS32 {
		t.Errorf("Class = %v, want %v (32-bit)", f.Class, elf.ELFCLASS32)
	}
	if f.Machine != elf.EM_ARM {
		t.Errorf("Machine = %v, want %v (arm)", f.Machine, elf.EM_ARM)
	}

	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			t.Errorf("binary has a PT_INTERP segment, meaning it needs a dynamic linker; want a statically linked binary")
		}
	}
}

// TestCrossCompileRecognizesLinuxOnlyMainPackage guards against a real bug
// found while adding examples/gpioinfo (bean gosd-nyad): its dependency on
// go-gpiocdev forces a `//go:build linux` tag on the example itself, and
// requireMainPackage's `go list` used to run under the host's own GOOS, so
// on a macOS host it saw "no Go files" and rejected a perfectly valid main
// package. CrossCompile always targets linux (targetGOOS), so its own
// preflight check must inspect the package as linux too.
func TestCrossCompileRecognizesLinuxOnlyMainPackage(t *testing.T) {
	out := filepath.Join(t.TempDir(), "linuxonly")

	if err := CrossCompile("./testdata/linuxonly", out, "", arm64); err != nil {
		t.Fatalf("CrossCompile: %v", err)
	}
}

func TestCrossCompileRejectsNonMainPackage(t *testing.T) {
	out := filepath.Join(t.TempDir(), "notmain")

	err := CrossCompile("./testdata/notmain", out, "", arm64)
	if err == nil {
		t.Fatal("CrossCompile succeeded on a non-main package, want an error")
	}
}

func TestCrossCompileSurfacesBuildFailure(t *testing.T) {
	err := CrossCompile("./testdata/doesnotexist", filepath.Join(t.TempDir(), "out"), "", arm64)
	if err == nil {
		t.Fatal("CrossCompile succeeded on a missing package, want an error")
	}
}

// TestCrossCompilePlacesTagsBeforePackagePath is the keystone test for
// gosd-1937: ./testdata/boardtag's default file only compiles when
// gosd_pi_zero_2w is absent, and it's written to fail to compile in that
// case, while the gosd_pi_zero_2w-tagged file is the only one that compiles
// cleanly. So CrossCompile with tags="gosd_pi_zero_2w" succeeding, and the
// same call with tags="" failing, together prove `-tags gosd_pi_zero_2w`
// reached `go build` (ahead of the package path - a `go build <pkg> -tags
// ...` invocation would silently ignore -tags as a package pattern).
func TestCrossCompilePlacesTagsBeforePackagePath(t *testing.T) {
	if err := CrossCompile("./testdata/boardtag", filepath.Join(t.TempDir(), "out"), "gosd_pi_zero_2w", arm64); err != nil {
		t.Errorf("CrossCompile with tags=gosd_pi_zero_2w failed: %v", err)
	}

	if err := CrossCompile("./testdata/boardtag", filepath.Join(t.TempDir(), "out"), "", arm64); err == nil {
		t.Error("CrossCompile with no tags succeeded, want the untagged fallback file's deliberate compile error to surface")
	}
}
