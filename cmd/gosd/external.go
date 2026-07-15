package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/staticelf"
)

// externalSpec is one parsed --with-external flag: a local file and the
// absolute path it lands at inside the initramfs.
type externalSpec struct {
	Path string
	Dest string
}

// splitExternalFlag splits a --with-external <path>[:<dest>] value on its
// last colon, but only when the text after that colon looks like an
// absolute initramfs path (starts with "/"). This lets a bare path (no
// colon at all) work, and keeps a Windows-style path such as
// `C:\tools\mpv.exe` - whose only colon is followed by a backslash, not a
// slash - from being mis-split into path "C" and a bogus dest.
func splitExternalFlag(raw string) (path, dest string) {
	i := strings.LastIndex(raw, ":")
	if i < 0 {
		return raw, ""
	}
	suffix := raw[i+1:]
	if !strings.HasPrefix(suffix, "/") {
		return raw, ""
	}
	return raw[:i], suffix
}

// reservedExternalDests are the exact initramfs paths a --with-external dest
// may never collide with; reservedExternalDestPrefixes are the namespaces
// (directories) it may never land inside.
var reservedExternalDests = map[string]string{
	"/init": "gosd-init",
	"/app":  "your app",
}

var reservedExternalDestPrefixes = map[string]string{
	"/etc/gosd/":     "gosd's reserved config directory",
	"/lib/firmware/": "gosd's reserved firmware directory",
}

// validateExternalDest rejects a --with-external dest that isn't an
// absolute initramfs path, or that collides with a path gosd itself
// controls.
func validateExternalDest(dest string) error {
	if !strings.HasPrefix(dest, "/") {
		return fmt.Errorf("--with-external dest %q is invalid because it must be an absolute path starting with \"/\"; try %q", dest, "/bin/"+filepath.Base(dest))
	}
	if owner, ok := reservedExternalDests[dest]; ok {
		return fmt.Errorf("--with-external dest %q is invalid because it collides with %s's own %s; choose a different dest", dest, owner, dest)
	}
	for prefix, desc := range reservedExternalDestPrefixes {
		if strings.HasPrefix(dest, prefix) {
			return fmt.Errorf("--with-external dest %q is invalid because it falls under %s (%s); choose a dest outside %s", dest, prefix, desc, prefix)
		}
	}
	return nil
}

// parseWithExternalFlags turns the repeated --with-external flag values
// into specs: a nil/empty flags always yields a nil slice and no error,
// since most builds never pass --with-external. Each dest defaults to
// "/bin/<basename of path>" and is validated (must be absolute, must not
// collide with a gosd-reserved path or another --with-external's dest)
// before the network/filesystem is ever touched.
func parseWithExternalFlags(flags []string) ([]externalSpec, error) {
	if len(flags) == 0 {
		return nil, nil
	}

	specs := make([]externalSpec, 0, len(flags))
	seenDest := make(map[string]bool, len(flags))
	for _, flag := range flags {
		path, dest := splitExternalFlag(flag)
		if path == "" {
			return nil, fmt.Errorf("--with-external %q is invalid because it has no path; use --with-external <path>[:<dest>]", flag)
		}
		if dest == "" {
			dest = "/bin/" + filepath.Base(path)
		}

		if err := validateExternalDest(dest); err != nil {
			return nil, err
		}
		if seenDest[dest] {
			return nil, fmt.Errorf("--with-external dest %q was given more than once; give each --with-external a distinct dest", dest)
		}
		seenDest[dest] = true

		specs = append(specs, externalSpec{Path: path, Dest: dest})
	}
	return specs, nil
}

// openExternalsForBoard opens a fresh reader for each of specs and
// pre-flights it against b's Arch (ELF class/machine match, no PT_INTERP -
// see validateStaticELF), so the one set of --with-external flags shared
// across every selected board can still be embedded independently in each
// board's own initramfs - pipeline.Assemble closes every reader it's handed
// once that board's build is done, so each board needs its own *os.File.
//
// If any file fails to open or fails pre-flight, every reader already
// opened in this call is closed before returning the error, so a partial
// failure never leaks file descriptors.
func openExternalsForBoard(specs []externalSpec, b boards.Board) (map[string]io.Reader, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	files := make(map[string]io.Reader, len(specs))
	for _, spec := range specs {
		f, err := os.Open(spec.Path)
		if err != nil {
			closeOpenedExternals(files)
			return nil, fmt.Errorf("opening --with-external file %s failed: %w; check the path exists and is readable", spec.Path, err)
		}
		if err := validateStaticELF(f, b); err != nil {
			_ = f.Close()
			closeOpenedExternals(files)
			return nil, err
		}
		files[spec.Dest] = f
	}
	return files, nil
}

func closeOpenedExternals(files map[string]io.Reader) {
	for _, r := range files {
		if c, ok := r.(io.Closer); ok {
			_ = c.Close()
		}
	}
}

// validateStaticELF pre-flights f (an opened --with-external file, read
// position at 0) against board b: it must parse as an ELF binary whose
// class/machine match b.Arch(), and it must have no PT_INTERP program
// header (i.e. it must be statically linked, since the gosd initramfs ships
// no ld.so or library layout for a dynamic loader to resolve against). The
// underlying ELF check lives in internal/staticelf (shared with
// internal/extbuild's post-build verification, bean gosd-sn30); this
// function's job is only to turn staticelf's generic error into
// --with-external/--board-specific, actionable wording. It only reads f's
// headers via io.ReaderAt (elf.NewFile never calls Read), so f's read
// position is untouched and the caller can still hand f to the pipeline as
// a fresh, unread reader afterwards.
func validateStaticELF(f *os.File, b boards.Board) error {
	err := staticelf.Verify(f, f.Name(), b.Arch())
	if err == nil {
		return nil
	}

	switch e := err.(type) {
	case *staticelf.NotELFError:
		return fmt.Errorf("--with-external file %s is not a valid ELF binary (%w); gosd build only bundles compiled static executables, not scripts or archives", f.Name(), e.Err)
	case *staticelf.MismatchError:
		return fmt.Errorf("--with-external file %s is a %s/%s binary, but --board %s needs %s/%s; cross-compile it with GOOS=linux GOARCH=%s%s (or drop --board %s)",
			f.Name(), e.GotClass, e.GotMachine, b.Name(), e.WantClass, e.WantMachine, b.Arch().GOARCH, staticelf.GOARMSuffix(b.Arch()), b.Name())
	case *staticelf.DynamicallyLinkedError:
		return fmt.Errorf("--with-external file %s is dynamically linked (it has a PT_INTERP program header requesting a dynamic loader); the gosd initramfs has no ld.so or library layout, so bundled binaries must be fully static - rebuild it with CGO_ENABLED=0 (Go) or full static linking (C/C++)", f.Name())
	default:
		return err
	}
}
