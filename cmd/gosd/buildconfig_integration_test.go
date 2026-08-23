package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
)

// writeTestAppRepo lays out what gosd-mwct's flagship flow assumes: an app
// repository — a Go module with a main package at ./app — carrying a
// gosd-build.toml. The module matters: `go list` resolves the app package
// through the working directory's module, exactly as it does for a real
// user running a bare `gosd build` in their checkout.
func writeTestAppRepo(t *testing.T, dir, buildConfig string) {
	t.Helper()
	for path, contents := range map[string]string{
		"go.mod":                        "module buildconfigtest\n\ngo 1.24\n",
		filepath.Join("app", "main.go"): "package main\n\nfunc main() {}\n",
		defaultBuildConfigFile:          buildConfig,
	} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// absFakeArtifacts resolves the fixture path before a test chdirs away from
// cmd/gosd.
func absFakeArtifacts(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("testdata/fake-artifacts")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func bootLabelOf(t *testing.T, imgPath string) string {
	t.Helper()
	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()
	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	return strings.TrimSpace(fs.Label())
}

// TestBuildReadsCheckedInConfigFile is the acceptance test for gosd-mwct's
// flagship flow: a repository with a gosd-build.toml builds with a bare
// `gosd build` - no positional argument, no flags - and every option in the
// file lands, with the file's relative paths ([app].main included) resolved
// against the file's own directory.
func TestBuildReadsCheckedInConfigFile(t *testing.T) {
	disableNetwork(t)
	dir := t.TempDir()
	writeTestAppRepo(t, dir, `
board = ["pi-zero-2w"]
output = "app-from-file.img"
label-prefix = "fromf"
placeholder = ["seed.bin=4KiB"]
artifacts-dir = "`+absFakeArtifacts(t)+`"

[app]
main = "./app"
`)
	t.Chdir(dir)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"build"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("bare gosd build with a gosd-build.toml failed: %v", err)
	}

	imgPath := filepath.Join(dir, "app-from-file.img")
	if got := bootLabelOf(t, imgPath); got != "fromf-boot" {
		t.Errorf("boot label = %q, want the file's label-prefix applied (fromf-boot)", got)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "app-from-file.inject.json"))
	if err != nil {
		t.Fatalf("the file's placeholder should have produced an inject manifest: %v", err)
	}
	if !strings.Contains(string(manifest), "seed.bin") {
		t.Errorf("inject manifest %q does not mention the file's placeholder seed.bin", manifest)
	}
}

// TestBuildFlagsBeatConfigFile pins the precedence half of gosd-mwct: a
// flag given on the command line wins over the file per key, while keys
// the command line leaves alone still come from the file.
func TestBuildFlagsBeatConfigFile(t *testing.T) {
	disableNetwork(t)
	dir := t.TempDir()
	writeTestAppRepo(t, dir, `
board = ["pi-zero-2w"]
output = "app-from-file.img"
label-prefix = "fromf"
artifacts-dir = "`+absFakeArtifacts(t)+`"

[app]
main = "./app"
`)
	t.Chdir(dir)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"build", "--label-prefix", "cliv", "-o", "from-cli.img"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "app-from-file.img")); !os.IsNotExist(err) {
		t.Errorf("-o on the command line should beat the file's output; the file's path exists (stat err %v)", err)
	}
	if got := bootLabelOf(t, filepath.Join(dir, "from-cli.img")); got != "cliv-boot" {
		t.Errorf("boot label = %q, want the CLI's label-prefix (cliv-boot)", got)
	}
}

// TestBuildConfigFlagReadsFileFromElsewhere covers the monorepo shape: the
// working directory is the repo root, the app and its gosd-build.toml live
// in a subdirectory named by --build-config, and the file's relative paths
// resolve against the file's directory, never the working directory.
func TestBuildConfigFlagReadsFileFromElsewhere(t *testing.T) {
	disableNetwork(t)
	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "frob")
	for path, contents := range map[string]string{
		filepath.Join(root, "go.mod"):           "module buildconfigtest\n\ngo 1.24\n",
		filepath.Join(appDir, "app", "main.go"): "package main\n\nfunc main() {}\n",
		filepath.Join(appDir, defaultBuildConfigFile): `
board = ["pi-zero-2w"]
output = "out.img"
artifacts-dir = "` + absFakeArtifacts(t) + `"

[app]
main = "./app"
`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"build", "--build-config", filepath.Join(appDir, defaultBuildConfigFile)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --build-config failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(appDir, "out.img")); err != nil {
		t.Errorf("output should land relative to the config file's directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "out.img")); !os.IsNotExist(err) {
		t.Errorf("output must not land in the working directory (stat err %v)", err)
	}
}

// TestRunReadsConfigFileAndIgnoresBuildOnlyKeys proves both halves of the
// gosd run decision: the keys run's flags mirror are honoured (a bare
// `gosd run` works, the file's kernel param reaches qemu), and build-only
// keys - board, output, the [publish] table, data.filesystem - change
// nothing about the run.
func TestRunReadsConfigFileAndIgnoresBuildOnlyKeys(t *testing.T) {
	disableNetwork(t)
	argsFile := filepath.Join(t.TempDir(), "qemu-args.txt")
	fakeQemuBinary(t, argsFile)

	dir := t.TempDir()
	writeTestAppRepo(t, dir, `
board = ["pi-3b"]
output = "never.img"
artifacts-dir = "`+absFakeArtifacts(t)+`"

[app]
main = "./app"

[kernel]
param = ["gosdtest.marker=1"]

[data]
filesystem = "ext4"

[publish]
catalog = true
`)
	t.Chdir(dir)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("bare gosd run with a gosd-build.toml failed: %v", err)
	}

	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("fake qemu-system-aarch64 was never invoked: %v", err)
	}
	argsLine := string(got)
	if !strings.Contains(argsLine, "gosdtest.marker=1") {
		t.Errorf("qemu invocation = %q, want the file's kernel.param on the command line", argsLine)
	}
	if !strings.Contains(argsLine, "app-qemu-virt.img") {
		t.Errorf("qemu invocation = %q, want the qemu-virt image regardless of the file's board key", argsLine)
	}
	if _, err := os.Stat(filepath.Join(dir, "never.img")); !os.IsNotExist(err) {
		t.Errorf("gosd run must ignore the file's build-only output key (stat err %v)", err)
	}
}

// TestBuildAndRunWithoutAnyMainErrorActionably pins the no-file-no-argument
// error: it must name both remedies rather than cobra's bare arg-count
// message.
func TestBuildAndRunWithoutAnyMainErrorActionably(t *testing.T) {
	t.Chdir(t.TempDir())

	for _, sub := range []string{"build", "run"} {
		cmd := newRootCmd()
		cmd.SetArgs([]string{sub})
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("gosd %s with no argument and no gosd-build.toml succeeded", sub)
		}
		for _, want := range []string{"main package", `main = "./cmd/myapp"`, "gosd " + sub + " ."} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("gosd %s error = %q, want it to mention %q", sub, err.Error(), want)
			}
		}
	}
}
