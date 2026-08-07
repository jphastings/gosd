package diskfmt

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/diskfmt/ext4golden"
)

// TestFormatEXT4PassesRealE2fsck cross-checks FormatEXT4's superblock
// stamping against real e2fsprogs, in a container: e2fsck -fn must pass
// clean, and tune2fs -l / blkid must report exactly the label and UUID
// diskfmt.Inspect itself reads back.
//
// This is what makes the checksum handling in ext4format.go trustworthy.
// TestEXT4ChecksumMatchesGoldenSuperblock only proves ext4Checksum matches
// e2fsprogs' own crc32c on an UNMODIFIED golden image; this test proves a
// volume this package actually wrote (with a fresh UUID and label stamped
// in, primary and backup superblocks alike) is one real e2fsprogs — built
// independently of anything in this repo — accepts as clean, not just one
// this package agrees with itself about.
//
// It follows internal/container/smoke_test.go's precedent for a
// docker-dependent test that must never become a silent, flaky dependency of
// the default `go test ./...` run (CI's ubuntu-latest images typically have
// a live Docker daemon already): opt-in via GOSD_CONTAINER_SMOKE_TEST=1, and
// even then skipped (never failed) if no runtime turns out to be usable.
func TestFormatEXT4PassesRealE2fsck(t *testing.T) {
	if os.Getenv("GOSD_CONTAINER_SMOKE_TEST") != "1" {
		t.Skip("set GOSD_CONTAINER_SMOKE_TEST=1 to cross-check a stamped ext4 superblock against real e2fsprogs")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	rt, err := container.Detect(ctx, "internal/diskfmt ext4 cross-check", "")
	if err != nil {
		var notInstalled *container.NotInstalledError
		var daemonDown *container.DaemonDownError
		if errors.As(err, &notInstalled) || errors.As(err, &daemonDown) {
			t.Skipf("no live container runtime available: %v", err)
		}
		t.Fatalf("Detect: %v", err)
	}

	dir := homeStagedDir(t)
	path := filepath.Join(dir, "ext4.img")
	if err := createSizedFile(path, ext4golden.RawBytes); err != nil {
		t.Fatalf("creating the target image: %v", err)
	}

	const label = "GOSD-DATA"
	if err := FormatEXT4(path, label); err != nil {
		t.Fatalf("FormatEXT4: %v", err)
	}

	want, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if want.FS != EXT4 || want.Label != label {
		t.Fatalf("Inspect after FormatEXT4 = %+v, want {FS:ext4 Label:%s}", want, label)
	}

	runTool := func(name string, cmd []string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		err := rt.Run(ctx, container.RunSpec{
			Image:  container.KernelBuildImage,
			Mounts: []container.Mount{{HostPath: dir, ContainerPath: "/work", ReadOnly: true}},
			Cmd:    cmd,
			Stdout: &stdout,
			Stderr: &stderr,
		})
		if err != nil {
			t.Fatalf("%s: %v\nstdout:\n%s", name, err, stdout.String())
		}
		return stdout.String()
	}

	e2fsckOut := runTool("e2fsck -fn", []string{"e2fsck", "-fn", "/work/ext4.img"})
	t.Logf("e2fsck -fn:\n%s", e2fsckOut)

	tune2fsFields := parseColonFields(runTool("tune2fs -l", []string{"tune2fs", "-l", "/work/ext4.img"}))
	if got := tune2fsFields["Filesystem volume name"]; got != label {
		t.Errorf("tune2fs -l Filesystem volume name = %q, want %q", got, label)
	}
	if got := tune2fsFields["Filesystem UUID"]; got != want.UUID {
		t.Errorf("tune2fs -l Filesystem UUID = %q, want %q", got, want.UUID)
	}

	blkidFields := parseKeyValueLines(runTool("blkid -o export", []string{"blkid", "-o", "export", "/work/ext4.img"}))
	if got := blkidFields["LABEL"]; got != label {
		t.Errorf("blkid LABEL = %q, want %q", got, label)
	}
	if got := blkidFields["UUID"]; got != want.UUID {
		t.Errorf("blkid UUID = %q, want %q", got, want.UUID)
	}
	if got := blkidFields["TYPE"]; got != "ext4" {
		t.Errorf("blkid TYPE = %q, want ext4", got)
	}
}

// parseColonFields parses "Key:   value" lines (dumpe2fs/tune2fs -l's
// output format, column-aligned so the whitespace after the colon varies
// per key) into a map, trimming both sides.
func parseColonFields(output string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return fields
}

// parseKeyValueLines parses blkid -o export's "KEY=value" lines into a map.
func parseKeyValueLines(output string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[key] = val
	}
	return fields
}

// homeStagedDir returns a fresh temp directory under the user's home
// directory rather than t.TempDir()'s default location: macOS's
// os.TempDir() (/var/folders) is not shared into the colima/Docker Desktop
// VM, so a bind mount rooted there silently mounts empty (see CLAUDE.md,
// "Container bind mounts must stage under the user's home").
func homeStagedDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory available to stage a container bind mount: %v", err)
	}
	base := filepath.Join(home, ".cache")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("creating %s: %v", base, err)
	}
	dir, err := os.MkdirTemp(base, "gosd-ext4-crosscheck-*")
	if err != nil {
		t.Fatalf("creating a home-staged temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// createSizedFile creates an empty, sparse file of exactly sizeBytes, the
// same way backingFile does for a t.TempDir()-rooted path.
func createSizedFile(path string, sizeBytes int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := f.Truncate(sizeBytes); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
