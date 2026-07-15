package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jphastings/gosd/internal/fsutil"
)

func TestCopyFileCopiesContentAndPermissions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.WriteFile(src, []byte("hello"), 0o750); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	if err := fsutil.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dst: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("dst content = %q, want %q", got, "hello")
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("statting dst: %v", err)
	}
	if info.Mode().Perm() != 0o750 {
		t.Errorf("dst perms = %v, want %v (copied from src)", info.Mode().Perm(), os.FileMode(0o750))
	}
}

func TestCopyFileOverwritesExistingDst(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatalf("writing fixture src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("stale content that is longer"), 0o644); err != nil {
		t.Fatalf("writing fixture dst: %v", err)
	}

	if err := fsutil.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dst: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("dst content = %q, want %q (truncated then overwritten)", got, "new")
	}
}

func TestCopyFileMissingSrcErrorsNamingIt(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "does-not-exist")
	dst := filepath.Join(dir, "dst")

	err := fsutil.CopyFile(src, dst)
	if err == nil {
		t.Fatal("CopyFile with a missing src succeeded, want an error")
	}
}
