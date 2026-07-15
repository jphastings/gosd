// Package staticelf checks whether a binary is a fully static ELF
// executable matching a given board architecture. The gosd initramfs ships
// no ld.so or library layout, so every binary it runs — the compiled app,
// gosd-init, a `gosd build --with-external` binary, or a `gosd
// build-external` (internal/extbuild) output — must be statically linked
// and built for the target's GOARCH/GOARM.
//
// This logic originally lived only in cmd/gosd/external.go (bean gosd-ig4h);
// it was extracted here (bean gosd-sn30) so internal/extbuild's post-build
// verification and cmd/gosd's --with-external pre-flight share one
// implementation instead of two copies drifting apart. Callers that need
// specific, audience-facing wording (e.g. cmd/gosd's "--with-external"/
// "--board" phrasing) inspect the returned error's concrete type and format
// their own message from its fields; Verify's own error text is generic.
package staticelf

import (
	"debug/elf"
	"fmt"
	"io"

	"github.com/jphastings/gosd/internal/boards"
)

// Expectations returns the ELF class/machine a static binary must have to
// run on arch, e.g. arm64 needs ELFCLASS64/EM_AARCH64, and arm/GOARM=6
// (pi-zero-w) needs ELFCLASS32/EM_ARM. It errors for any GOARCH gosd
// doesn't yet know how to map, rather than silently skipping validation for
// it.
func Expectations(arch boards.Arch) (elf.Class, elf.Machine, error) {
	switch arch.GOARCH {
	case "arm64":
		return elf.ELFCLASS64, elf.EM_AARCH64, nil
	case "arm":
		return elf.ELFCLASS32, elf.EM_ARM, nil
	default:
		return 0, 0, fmt.Errorf("staticelf: no ELF class/machine mapping for GOARCH=%s; gosd needs updating to validate this target", arch.GOARCH)
	}
}

// GOARMSuffix returns " GOARM=<n>" for an arch that sets one (e.g.
// pi-zero-w's arm/GOARM=6), or "" for arches that don't (e.g. arm64) —
// useful for building the exact cross-compile env a developer needs in an
// actionable error message.
func GOARMSuffix(arch boards.Arch) string {
	if arch.GOARM == "" {
		return ""
	}
	return " GOARM=" + arch.GOARM
}

// Verify checks that r parses as an ELF binary whose class/machine match
// arch, and that it has no PT_INTERP program header (i.e. it is statically
// linked — a real dynamic loader would be requested via PT_INTERP). subject
// names what's being checked (typically a file path) for use in the
// returned error's fields; Verify does not read subject itself.
//
// It only reads r's headers (elf.NewFile never calls Read on anything but
// an io.ReaderAt), so a caller handing it an *os.File can keep using that
// file's untouched read position afterwards.
func Verify(r io.ReaderAt, subject string, arch boards.Arch) error {
	ef, err := elf.NewFile(r)
	if err != nil {
		return &NotELFError{Subject: subject, Err: err}
	}
	defer func() { _ = ef.Close() }()

	wantClass, wantMachine, err := Expectations(arch)
	if err != nil {
		return err
	}
	if ef.Class != wantClass || ef.Machine != wantMachine {
		return &MismatchError{
			Subject:     subject,
			Arch:        arch,
			GotClass:    ef.Class,
			GotMachine:  ef.Machine,
			WantClass:   wantClass,
			WantMachine: wantMachine,
		}
	}

	for _, p := range ef.Progs {
		if p.Type == elf.PT_INTERP {
			return &DynamicallyLinkedError{Subject: subject}
		}
	}

	return nil
}

// NotELFError means subject did not parse as a valid ELF binary at all.
type NotELFError struct {
	Subject string
	Err     error
}

func (e *NotELFError) Error() string {
	return fmt.Sprintf("%s is not a valid ELF binary: %v", e.Subject, e.Err)
}

func (e *NotELFError) Unwrap() error { return e.Err }

// MismatchError means subject parsed as ELF but its class/machine don't
// match the target arch.
type MismatchError struct {
	Subject                 string
	Arch                    boards.Arch
	GotClass, WantClass     elf.Class
	GotMachine, WantMachine elf.Machine
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf(
		"%s is a %s/%s binary, but GOARCH=%s%s needs %s/%s",
		e.Subject, e.GotClass, e.GotMachine, e.Arch.GOARCH, GOARMSuffix(e.Arch), e.WantClass, e.WantMachine,
	)
}

// DynamicallyLinkedError means subject has a PT_INTERP program header (it
// requests a dynamic loader), which gosd's initramfs has no ld.so or
// library layout to satisfy.
type DynamicallyLinkedError struct {
	Subject string
}

func (e *DynamicallyLinkedError) Error() string {
	return fmt.Sprintf(
		"%s is dynamically linked (it has a PT_INTERP program header requesting a dynamic loader); "+
			"gosd needs a fully static binary since its initramfs ships no ld.so or library layout",
		e.Subject,
	)
}
