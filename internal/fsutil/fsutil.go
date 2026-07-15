// Package fsutil holds small filesystem helpers shared by gosd's container-
// build packages (internal/kernelbuild, internal/extbuild) and the CLI
// commands that drive them, so a plain byte-for-byte file copy - out of a
// content-addressed cache entry and into a caller-chosen output location -
// has exactly one implementation instead of being hand-rolled per package.
package fsutil

import (
	"fmt"
	"io"
	"os"
)

// CopyFile copies src to dst, preserving src's permission bits. dst is
// created if it doesn't exist and truncated if it does; dst's parent
// directory must already exist (callers that need it created first do so
// themselves, since directory-creation policy - e.g. whether it's an error
// for the parent to be missing - varies per caller).
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	return out.Close()
}
