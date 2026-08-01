package pipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/klauspost/compress/zstd"
	"github.com/u-root/u-root/pkg/cpio"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/pipeline"
)

// fakeBoard is a minimal boards.Board that records what the pipeline passes
// it, so tests can assert on build order and data flow without needing a
// real cross-compiled binary or firmware manifest.
type fakeBoard struct {
	name         string
	firmware     map[string]io.Reader
	rawWrites    []image.RawWrite
	bootFilesErr error

	gotConfig    boards.BuildConfig
	gotInitramfs io.Reader
}

func (b *fakeBoard) Name() string                    { return b.name }
func (b *fakeBoard) Arch() boards.Arch               { return boards.Arch{GOARCH: "arm64"} }
func (b *fakeBoard) Artifacts() []boards.ArtifactRef { return nil }

func (b *fakeBoard) BootFiles(cfg boards.BuildConfig, art boards.Artifacts) (map[string]io.Reader, error) {
	b.gotConfig = cfg
	b.gotInitramfs = art.Initramfs
	if b.bootFilesErr != nil {
		return nil, b.bootFilesErr
	}
	return map[string]io.Reader{"initramfs.cpio.zst": art.Initramfs}, nil
}

func (b *fakeBoard) RawWrites(boards.Artifacts) []image.RawWrite { return b.rawWrites }

func (b *fakeBoard) FirmwareFiles(boards.Artifacts) map[string]io.Reader { return b.firmware }

func (b *fakeBoard) UsbGadgetSupport() boards.GadgetSupport {
	return boards.GadgetSupport{Supported: true}
}

func (b *fakeBoard) ConsoleBaudSupport() boards.ConsoleBaudSupport {
	return boards.ConsoleBaudSupport{Supported: true}
}

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return p
}

func TestAssembleBuildsInitramfsBeforeCallingBootFiles(t *testing.T) {
	dir := t.TempDir()
	appPath := writeTempFile(t, dir, "app", "app binary bytes")
	initPath := writeTempFile(t, dir, "gosd-init", "init binary bytes")

	b := &fakeBoard{
		name:     "fake-board",
		firmware: map[string]io.Reader{"wifi.bin": strings.NewReader("wifi bytes")},
	}

	imgPath := filepath.Join(dir, "out.img")
	_, err := pipeline.Assemble(context.Background(), pipeline.Options{
		Board:          b,
		AppBinaryPath:  appPath,
		InitBinaryPath: initPath,
		Config:         boards.BuildConfig{Hostname: "myhost", HostnameExplicit: true, WifiSSID: "ssid", WifiPassword: "pass"},
		OutputPath:     imgPath,
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if b.gotConfig.Hostname != "myhost" {
		t.Errorf("Board.BootFiles got Config.Hostname = %q, want myhost", b.gotConfig.Hostname)
	}
	if b.gotInitramfs == nil {
		t.Fatal("Board.BootFiles was called with a nil initramfs; want the pipeline to build it first")
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the image: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1): %v", err)
	}

	raw, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst back: %v", err)
	}
	records := decodeInitramfs(t, raw)

	assertRecordContent(t, records, "init", "init binary bytes")
	assertRecordContent(t, records, "app", "app binary bytes")
	assertRecordContent(t, records, "lib/firmware/wifi.bin", "wifi bytes")

	config := recordContent(t, records, "etc/gosd/config.json")
	for _, want := range []string{`"hostname":"myhost"`, `"ssid":"ssid"`, `"passphrase":"pass"`, `"board":"fake-board"`} {
		if !strings.Contains(string(config), want) {
			t.Errorf("config.json = %s, want it to contain %q", config, want)
		}
	}

	gosdToml, err := fs.ReadFile("gosd.toml")
	if err != nil {
		t.Fatalf("reading gosd.toml back from the FAT root: %v", err)
	}
	for _, want := range []string{`hostname = "myhost"`, `ssid = "ssid"`, `passphrase = "pass"`} {
		if !strings.Contains(string(gosdToml), want) {
			t.Errorf("gosd.toml = %s, want it to contain %q", gosdToml, want)
		}
	}
}

func TestAssembleBakesDataExpandIntoConfigJSON(t *testing.T) {
	dir := t.TempDir()
	appPath := writeTempFile(t, dir, "app", "app")
	initPath := writeTempFile(t, dir, "gosd-init", "init")

	imgPath := filepath.Join(dir, "out.img")
	_, err := pipeline.Assemble(context.Background(), pipeline.Options{
		Board: &fakeBoard{name: "fake-board"}, AppBinaryPath: appPath, InitBinaryPath: initPath,
		OutputPath: imgPath,
		DataExpand: true,
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the image: %v", err)
	}
	defer func() { _ = d.Close() }()

	// The expand flag must not create an image-side partition: that's the
	// point of it.
	if part2, err := d.GetPartition(2); err == nil && part2.GetSize() != 0 {
		t.Errorf("partition 2 has size %d with DataExpand, want none in the image", part2.GetSize())
	}

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1): %v", err)
	}
	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	config := recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json")
	if !strings.Contains(string(config), `"dataExpand":true`) {
		t.Errorf("config.json = %s, want it to contain %q", config, `"dataExpand":true`)
	}
}

// TestAssembleBakesBuildTimestampIntoConfigJSON confirms config.json carries
// a fresh, parseable build timestamp — timesync's clock floor (gosd-0esw).
// See TestBuildIdentityIsReproducibleAcrossRebuilds (build_integration_test.go)
// for the companion proof that a value which necessarily differs on every
// build never moves the image identity.
func TestAssembleBakesBuildTimestampIntoConfigJSON(t *testing.T) {
	dir := t.TempDir()
	appPath := writeTempFile(t, dir, "app", "app")
	initPath := writeTempFile(t, dir, "gosd-init", "init")

	before := time.Now().Add(-time.Minute)
	imgPath := filepath.Join(dir, "out.img")
	_, err := pipeline.Assemble(context.Background(), pipeline.Options{
		Board: &fakeBoard{name: "fake-board"}, AppBinaryPath: appPath, InitBinaryPath: initPath,
		OutputPath: imgPath,
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	after := time.Now().Add(time.Minute)

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the image: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1): %v", err)
	}
	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	configJSON := recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json")

	var cfg initcfg.Config
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		t.Fatalf("config.json = %s is not valid JSON: %v", configJSON, err)
	}

	got := cfg.BuildTime()
	if got.IsZero() {
		t.Fatalf("config.json's buildTimestamp = %q, want a parseable RFC3339Nano timestamp", cfg.BuildTimestamp)
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("config.json's buildTimestamp = %s, want it between %s and %s (the Assemble call)", got, before, after)
	}
}

func TestAssembleBakesEnvIntoConfigJSONAndGosdToml(t *testing.T) {
	dir := t.TempDir()
	appPath := writeTempFile(t, dir, "app", "app")
	initPath := writeTempFile(t, dir, "gosd-init", "init")

	b := &fakeBoard{name: "fake-board"}
	imgPath := filepath.Join(dir, "out.img")
	_, err := pipeline.Assemble(context.Background(), pipeline.Options{
		Board: b, AppBinaryPath: appPath, InitBinaryPath: initPath,
		Config:     boards.BuildConfig{Env: map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "debug"}},
		OutputPath: imgPath,
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the image: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1): %v", err)
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	records := decodeInitramfs(t, initramfsBytes)
	config := recordContent(t, records, "etc/gosd/config.json")
	for _, want := range []string{`"API_URL":"https://example.com"`, `"LOG_LEVEL":"debug"`} {
		if !strings.Contains(string(config), want) {
			t.Errorf("config.json = %s, want it to contain %q", config, want)
		}
	}

	gosdToml, err := fs.ReadFile("gosd.toml")
	if err != nil {
		t.Fatalf("reading gosd.toml back from the FAT root: %v", err)
	}
	for _, want := range []string{`API_URL = "https://example.com"`, `LOG_LEVEL = "debug"`} {
		if !strings.Contains(string(gosdToml), want) {
			t.Errorf("gosd.toml = %s, want it to contain %q", gosdToml, want)
		}
	}
}

func TestAssembleWritesCommentedGosdTomlWhenConfigUnset(t *testing.T) {
	dir := t.TempDir()
	appPath := writeTempFile(t, dir, "app", "app")
	initPath := writeTempFile(t, dir, "gosd-init", "init")

	b := &fakeBoard{name: "fake-board"}
	imgPath := filepath.Join(dir, "out.img")
	if _, err := pipeline.Assemble(context.Background(), pipeline.Options{
		Board: b, AppBinaryPath: appPath, InitBinaryPath: initPath, OutputPath: imgPath,
	}); err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the image: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1): %v", err)
	}

	gosdToml, err := fs.ReadFile("gosd.toml")
	if err != nil {
		t.Fatalf("reading gosd.toml back from the FAT root: %v", err)
	}
	if !strings.Contains(string(gosdToml), `# hostname = "my-device"`) {
		t.Errorf("gosd.toml = %s, want a commented-out hostname example when unset", gosdToml)
	}
}

// TestAssembleWritesCommentedGosdTomlHostnameForNonExplicitDefault is the
// core regression test for bean gosd-4hz1: a build's sanitized-default
// hostname (HostnameExplicit left false, as `gosd build` leaves it when
// --hostname isn't passed) must still land in config.json as the baked
// fallback, but must NOT be baked uncommented into gosd.toml - otherwise it
// always shadows an Imager wizard's cloud-init hostname, since gosd.toml
// outranks cloud-init in the locked precedence chain.
func TestAssembleWritesCommentedGosdTomlHostnameForNonExplicitDefault(t *testing.T) {
	dir := t.TempDir()
	appPath := writeTempFile(t, dir, "app", "app")
	initPath := writeTempFile(t, dir, "gosd-init", "init")

	b := &fakeBoard{name: "fake-board"}
	imgPath := filepath.Join(dir, "out.img")
	if _, err := pipeline.Assemble(context.Background(), pipeline.Options{
		Board: b, AppBinaryPath: appPath, InitBinaryPath: initPath, OutputPath: imgPath,
		Config: boards.BuildConfig{Hostname: "sanitized-default"},
	}); err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the image: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1): %v", err)
	}

	gosdToml, err := fs.ReadFile("gosd.toml")
	if err != nil {
		t.Fatalf("reading gosd.toml back from the FAT root: %v", err)
	}
	if !strings.Contains(string(gosdToml), `# hostname = "sanitized-default"`) {
		t.Errorf("gosd.toml = %s, want the default hostname shown as a commented-out example", gosdToml)
	}
	if strings.Contains(string(gosdToml), "\nhostname = ") {
		t.Errorf("gosd.toml = %s, want no uncommented hostname line for a non-explicit default hostname", gosdToml)
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	config := recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json")
	if !strings.Contains(string(config), `"hostname":"sanitized-default"`) {
		t.Errorf("config.json = %s, want it to still carry the default hostname as the baked fallback", config)
	}
}

func TestAssembleAppliesRawWrites(t *testing.T) {
	dir := t.TempDir()
	appPath := writeTempFile(t, dir, "app", "app")
	initPath := writeTempFile(t, dir, "gosd-init", "init")

	rawContent := []byte("bootloader-payload")
	b := &fakeBoard{
		name:      "fake-board",
		rawWrites: []image.RawWrite{{OffsetBytes: 64 * 512, Content: bytes.NewReader(rawContent)}},
	}

	imgPath := filepath.Join(dir, "out.img")
	if _, err := pipeline.Assemble(context.Background(), pipeline.Options{
		Board: b, AppBinaryPath: appPath, InitBinaryPath: initPath, OutputPath: imgPath,
	}); err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the image: %v", err)
	}
	defer func() { _ = d.Close() }()

	got := make([]byte, len(rawContent))
	if _, err := d.Backend.ReadAt(got, 64*512); err != nil {
		t.Fatalf("reading back the raw write: %v", err)
	}
	if !bytes.Equal(got, rawContent) {
		t.Errorf("raw write content = %q, want %q", got, rawContent)
	}
}

func TestAssembleThreadsDataSizeBytesIntoTheImage(t *testing.T) {
	dir := t.TempDir()
	appPath := writeTempFile(t, dir, "app", "app")
	initPath := writeTempFile(t, dir, "gosd-init", "init")

	b := &fakeBoard{name: "fake-board"}
	imgPath := filepath.Join(dir, "out.img")
	const dataSizeBytes = 4 * 1024 * 1024
	if _, err := pipeline.Assemble(context.Background(), pipeline.Options{
		Board: b, AppBinaryPath: appPath, InitBinaryPath: initPath,
		OutputPath: imgPath, DataSizeBytes: dataSizeBytes,
	}); err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the image: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(2)
	if err != nil {
		t.Fatalf("GetFilesystem(2): %v", err)
	}
	if label := strings.TrimSpace(fs.Label()); label != "GOSD-DATA" {
		t.Errorf("partition 2 label = %q, want GOSD-DATA", label)
	}
}

func TestAssembleSurfacesBoardBootFilesError(t *testing.T) {
	dir := t.TempDir()
	appPath := writeTempFile(t, dir, "app", "app")
	initPath := writeTempFile(t, dir, "gosd-init", "init")

	wantErr := errors.New("board-specific boot files failure")
	b := &fakeBoard{name: "fake-board", bootFilesErr: wantErr}

	_, err := pipeline.Assemble(context.Background(), pipeline.Options{
		Board: b, AppBinaryPath: appPath, InitBinaryPath: initPath,
		OutputPath: filepath.Join(dir, "out.img"),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Assemble() error = %v, want it to wrap %v", err, wantErr)
	}
}

func decodeInitramfs(t *testing.T, compressed []byte) []cpio.Record {
	t.Helper()

	zr, err := zstd.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("creating zstd reader: %v", err)
	}
	defer zr.Close()

	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompressing initramfs: %v", err)
	}

	records, err := cpio.ReadAllRecords(cpio.Newc.Reader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("reading cpio records: %v", err)
	}
	return records
}

func recordContent(t *testing.T, records []cpio.Record, name string) []byte {
	t.Helper()
	for _, r := range records {
		if r.Name != name {
			continue
		}
		got := make([]byte, r.FileSize)
		if _, err := r.ReadAt(got, 0); err != nil && err != io.EOF {
			t.Fatalf("reading record %q: %v", name, err)
		}
		return got
	}
	t.Fatalf("no record named %q found", name)
	return nil
}

func assertRecordContent(t *testing.T, records []cpio.Record, name, want string) {
	t.Helper()
	if got := string(recordContent(t, records, name)); got != want {
		t.Errorf("record %q content = %q, want %q", name, got, want)
	}
}
