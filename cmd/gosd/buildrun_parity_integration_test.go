package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/u-root/u-root/pkg/cpio"
)

// TestBuildAndRunProduceIdenticalInitramfsContent is gosd-x3j5's durable
// guard: `gosd build --board=qemu-virt` and `gosd run` (the only board it
// ever builds) each construct their own pipeline.Options, so any image
// content wired into one path but not the other - the CA bundle nearly
// shipped build-only, bean gosd-kzgq - silently diverges a qemu image from
// a flashed one with nothing to notice until someone diffs the two by hand.
// With matching flags and the same fake artifacts, both must produce the
// exact same initramfs entries, byte-for-byte, except
// etc/gosd/config.json's baked build timestamp (initcfg.Config.BuildTimestamp
// - the one field that's expected, by design, to differ between any two
// builds run moments apart).
func TestBuildAndRunProduceIdenticalInitramfsContent(t *testing.T) {
	disableNetwork(t)

	buildImg := filepath.Join(t.TempDir(), "hello-qemu-virt.img")
	buildCmd := newRootCmd()
	buildCmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "qemu-virt",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--hostname", "parity-test",
		"--ingress", "cloudflared",
		"--ingress", "tailscale-funnel",
		"--data-size", "64MiB",
		"-o", buildImg,
	})
	if err := buildCmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=qemu-virt failed: %v", err)
	}

	argsFile := filepath.Join(t.TempDir(), "qemu-args.txt")
	fakeQemuBinary(t, argsFile)

	var runStderr bytes.Buffer
	runCmd := newRootCmd()
	runCmd.SetErr(&runStderr)
	runCmd.SetArgs([]string{
		"run", "../../examples/hello",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--hostname", "parity-test",
		"--ingress", "cloudflared",
		"--ingress", "tailscale-funnel",
		"--data-size", "64MiB",
		"--keep",
	})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("gosd run failed: %v", err)
	}
	runDir := extractKeptPath(t, runStderr.String())
	defer func() { _ = os.RemoveAll(runDir) }()
	runImg := filepath.Join(runDir, "hello-qemu-virt.img")

	buildRecords := decodeInitramfs(t, readBootFile(t, buildImg, "initramfs.cpio.zst"))
	runRecords := decodeInitramfs(t, readBootFile(t, runImg, "initramfs.cpio.zst"))

	// Both sides must actually carry the --ingress content, not just agree
	// by both omitting it - see sharedcontent.go's doc comment for why the
	// CA bundle (and now cloudflared) is routed through one shared path
	// specifically to prevent a build-only regression like this; gosd-kzd3
	// extends the same guarantee to tailscale-funnel's per-arch-compiled
	// shim, which reaches build.go and run.go through compileForBoards
	// rather than sharedcontent.go, so this is the test that would catch a
	// build.go-only (or run.go-only) wiring mistake for it.
	for _, records := range [][]cpio.Record{buildRecords, runRecords} {
		if !hasRecord(records, "bin/cloudflared") {
			t.Fatalf("--ingress cloudflared build is missing bin/cloudflared; got entries %v", recordNames(records))
		}
		if !hasRecord(records, "bin/gosd-tsfunnel") {
			t.Fatalf("--ingress tailscale-funnel build is missing bin/gosd-tsfunnel; got entries %v", recordNames(records))
		}
	}

	assertIdenticalInitramfsContent(t, buildRecords, runRecords)
}

// readBootFile reopens the .img at imgPath and reads name back from its
// boot partition (partition 1), the same way every build-integration test
// verifies boot-partition content.
func readBootFile(t *testing.T, imgPath, name string) []byte {
	t.Helper()

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening %s failed: %v", imgPath, err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) for %s failed: %v", imgPath, err)
	}

	content, err := fs.ReadFile(name)
	if err != nil {
		t.Fatalf("reading %s from %s failed: %v", name, imgPath, err)
	}
	return content
}

// varyingInitramfsEntry is the one initramfs entry allowed to differ between
// otherwise-identical builds: config.json bakes in the wall-clock build
// timestamp (initcfg.Config.BuildTimestamp), by design, so every other
// build/run invocation of the same content changes this file's bytes even
// when nothing else about the image does.
const varyingInitramfsEntry = "etc/gosd/config.json"

// assertIdenticalInitramfsContent fails the test if buildRecords and
// runRecords don't carry the exact same set of entries, at the same mode
// and content, aside from varyingInitramfsEntry.
func assertIdenticalInitramfsContent(t *testing.T, buildRecords, runRecords []cpio.Record) {
	t.Helper()

	buildNames, runNames := recordNames(buildRecords), recordNames(runRecords)
	sort.Strings(buildNames)
	sort.Strings(runNames)
	if !equalStrings(buildNames, runNames) {
		t.Fatalf("initramfs entries differ between gosd build --board=qemu-virt and gosd run:\nbuild: %v\nrun:   %v", buildNames, runNames)
	}

	for _, name := range buildNames {
		if name == varyingInitramfsEntry {
			continue
		}

		buildRec, _ := findRecord(buildRecords, name)
		runRec, _ := findRecord(runRecords, name)
		if buildRec.Mode != runRec.Mode {
			t.Errorf("%s: mode differs between gosd build (%#o) and gosd run (%#o)", name, buildRec.Mode, runRec.Mode)
		}

		buildContent := recordContent(t, buildRecords, name)
		runContent := recordContent(t, runRecords, name)
		if !bytes.Equal(buildContent, runContent) {
			t.Errorf("%s: content differs between gosd build --board=qemu-virt and gosd run", name)
		}
	}
}
