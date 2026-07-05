// Package initramfs builds the initramfs cpio archive that gosd-init and
// the boot process run out of: gosd-init as /init, the user app as /app,
// per-board firmware under /lib/firmware, and the build-time config at
// /etc/gosd/config.json.
//
// Archives are written in newc cpio format (github.com/u-root/u-root/pkg/cpio)
// and zstd-compressed (github.com/klauspost/compress/zstd). Output is
// deterministic: every entry has a zeroed mtime/uid/gid, entries are written
// in sorted path order, and parent directories are synthesized explicitly
// (the Linux kernel's initramfs unpacker ignores a file if its parent
// directory has no entry of its own).
package initramfs

import (
	"fmt"
	"io"
	"os"
	"path"
	"sort"

	"github.com/klauspost/compress/zstd"
	"github.com/u-root/u-root/pkg/cpio"
)

// dirMode is the permission bits used for directories synthesized to hold
// nested files, e.g. "lib" and "lib/firmware" for a "/lib/firmware/x.bin"
// entry.
const dirMode = 0o755

// File describes one file to embed in the initramfs archive.
type File struct {
	// Path is the file's absolute path inside the archive, e.g. "/init".
	Path string
	// Content supplies the file's bytes.
	Content io.Reader
	// Mode is the file's permission bits, e.g. 0o755.
	Mode os.FileMode
}

// Spec lists every file the initramfs archive should contain. Parent
// directories are inferred automatically from the given paths and must not
// be listed separately.
type Spec struct {
	Files []File

	// Dirs lists additional empty directories to create, e.g. mount
	// points gosd-init needs to already exist before it can mount
	// something there (mount(2) fails with ENOENT on a missing target,
	// and this rootfs starts out containing nothing but what this
	// package writes — the kernel does not create /proc, /sys, or /run
	// for you). Entries that are already implied by a File's path are
	// harmless duplicates, not errors.
	Dirs []string
}

// Build writes spec's files, plus their inferred parent directories, to w as
// a zstd-compressed newc cpio archive.
//
// Calling Build twice with equivalent Specs (same paths, contents, and
// modes) always produces byte-identical output.
func Build(w io.Writer, spec Spec) error {
	records, err := buildRecords(spec)
	if err != nil {
		return err
	}

	zw, err := zstd.NewWriter(w, zstd.WithEncoderConcurrency(1))
	if err != nil {
		return fmt.Errorf("initramfs: creating zstd writer: %w", err)
	}

	cw := cpio.Newc.Writer(zw)
	for _, r := range records {
		if err := cw.WriteRecord(r); err != nil {
			_ = zw.Close()
			return fmt.Errorf("initramfs: writing %q: %w", r.Name, err)
		}
	}
	if err := cpio.WriteTrailer(cw); err != nil {
		_ = zw.Close()
		return fmt.Errorf("initramfs: writing archive trailer: %w", err)
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("initramfs: closing zstd writer: %w", err)
	}
	return nil
}

type entryKind int

const (
	kindFile entryKind = iota
	kindDir
)

type node struct {
	kind    entryKind
	mode    os.FileMode
	content []byte
}

// buildRecords turns spec into a sorted, deterministic list of cpio records,
// synthesizing any parent directories the given file paths require.
func buildRecords(spec Spec) ([]cpio.Record, error) {
	nodes := make(map[string]*node, len(spec.Files)*2)
	order := make([]string, 0, len(spec.Files)*2)

	add := func(name string, n node) error {
		if existing, ok := nodes[name]; ok {
			if existing.kind != n.kind {
				return fmt.Errorf("initramfs: %q is used as both a file and a directory", name)
			}
			return nil
		}
		nodes[name] = &n
		order = append(order, name)
		return nil
	}

	for _, f := range spec.Files {
		name := cpio.Normalize(f.Path)
		if name == "" || name == "." {
			return nil, fmt.Errorf("initramfs: invalid file path %q", f.Path)
		}
		if _, ok := nodes[name]; ok {
			return nil, fmt.Errorf("initramfs: duplicate path %q", f.Path)
		}

		var content []byte
		if f.Content != nil {
			b, err := io.ReadAll(f.Content)
			if err != nil {
				return nil, fmt.Errorf("initramfs: reading content for %q: %w", f.Path, err)
			}
			content = b
		}
		if err := add(name, node{kind: kindFile, mode: f.Mode, content: content}); err != nil {
			return nil, err
		}

		for dir := path.Dir(name); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if err := add(dir, node{kind: kindDir, mode: dirMode}); err != nil {
				return nil, err
			}
		}
	}

	for _, d := range spec.Dirs {
		name := cpio.Normalize(d)
		if name == "" || name == "." {
			return nil, fmt.Errorf("initramfs: invalid directory path %q", d)
		}
		for dir := name; dir != "." && dir != "/"; dir = path.Dir(dir) {
			if err := add(dir, node{kind: kindDir, mode: dirMode}); err != nil {
				return nil, err
			}
		}
	}

	sort.Strings(order)

	records := make([]cpio.Record, 0, len(order))
	for _, name := range order {
		n := nodes[name]
		info := cpio.Info{
			Name:  name,
			Mode:  uint64(n.mode.Perm()),
			NLink: 1,
		}
		switch n.kind {
		case kindFile:
			info.Mode |= cpio.S_IFREG
		case kindDir:
			info.Mode |= cpio.S_IFDIR
		}
		records = append(records, cpio.StaticRecord(n.content, info))
	}
	return records, nil
}
