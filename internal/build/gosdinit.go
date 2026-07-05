package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

// GosdModulePath is gosd's own module path. It's used to recognize when a
// working directory or a running gosd binary belongs to gosd's own source
// tree, as opposed to a downstream app that merely depends on gosd.
const GosdModulePath = "github.com/jphastings/gosd"

// gosdInitRelPkg is where cmd/gosd-init lives relative to the root of
// whichever copy of the gosd module we end up building it from.
const gosdInitRelPkg = "./cmd/gosd-init"

// CrossCompileGosdInit locates gosd's own gosd-init source and cross-compiles
// it to outputPath. It exists because `gosd build` needs to bake a
// cross-compiled gosd-init into every image, but gosd-init's source isn't
// necessarily available on disk: a developer who `go install`s gosd and runs
// it inside their own app's repo has no local copy of gosd's own source.
//
// overrideDir, when non-empty, is the --gosd-init-src escape hatch (rung 3
// below) and is used as-is, skipping detection entirely.
//
// Otherwise, source is located by a two-rung ladder:
//
//  1. Dev workflow, unchanged: if the module rooted at the current working
//     directory is gosd itself, or this very gosd binary was compiled from a
//     checkout that's still present on disk (recognized via this file's own
//     compile-time source path), gosd-init is built straight from that
//     checkout. This is the path taken when hacking on gosd itself, or when
//     running a locally-built gosd binary from anywhere on the same machine.
//  2. go-installed elsewhere: otherwise, gosd resolves its own build version
//     via runtime/debug.ReadBuildInfo and asks `go mod download` to
//     fetch (or find already-cached) github.com/jphastings/gosd at that
//     exact version, then builds gosd-init from the resulting module cache
//     directory. This needs network access only the first time for a given
//     version; the module cache serves every build after that, same as
//     gosd's other artifact caching.
func CrossCompileGosdInit(outputPath, overrideDir string) error {
	if overrideDir != "" {
		return crossCompileInDir(overrideDir, ".", outputPath,
			fmt.Sprintf("--gosd-init-src %s", overrideDir))
	}

	if dir, ok := devCheckoutDir(); ok {
		return crossCompileInDir(dir, gosdInitRelPkg, outputPath,
			fmt.Sprintf("local checkout at %s", dir))
	}

	dir, err := moduleCacheDir()
	if err != nil {
		return err
	}
	return crossCompileInDir(dir, gosdInitRelPkg, outputPath,
		fmt.Sprintf("module cache at %s", dir))
}

// crossCompileInDir builds relPkg (a package path relative to dir) as found
// in dir, writing the result to outputPath. It runs `go` with `-C dir`
// rather than cd-ing the current process into dir, so it works regardless of
// gosd's own working directory, and never writes into dir itself (dir may be
// a read-only module cache entry).
func crossCompileInDir(dir, relPkg, outputPath, source string) error {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("gosd-init source directory %s (%s) does not exist; try passing --gosd-init-src <dir>", dir, source)
	}

	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolving output path %s: %w", outputPath, err)
	}

	cmd := exec.Command("go", "-C", dir, "build", "-o", absOutput, relPkg)
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+targetGOOS,
		"GOARCH="+targetGOARCH,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"building gosd-init from %s failed; try running `go -C %s build %s` directly to reproduce:\n%s",
			source, dir, relPkg, stderr.String())
	}
	return nil
}

// devCheckoutDir implements rung 1: it reports the root of a gosd checkout
// still present on disk, either because the current working directory
// belongs to gosd itself, or because gosd was compiled from a checkout that
// hasn't moved since.
func devCheckoutDir() (string, bool) {
	if dir, ok := moduleRootForModule(""); ok {
		return dir, true
	}

	// Fall back to the source path the Go compiler embedded in this very
	// binary at build time. Unless gosd was built with -trimpath, this
	// still points at the checkout it was built from, even when gosd is
	// later run from a completely different working directory - which is
	// exactly the case of a locally-built gosd used against another repo.
	if _, file, _, ok := runtime.Caller(0); ok {
		if dir, ok := moduleRootForModule(filepath.Dir(file)); ok {
			return dir, true
		}
	}

	return "", false
}

// moduleRootForModule reports the root directory of the gosd module as seen
// from dir (the empty string meaning gosd's own current working directory),
// but only if that module really is gosd and it still has a cmd/gosd-init
// under it. Any failure (no such directory, no go.mod, a different module,
// missing cmd/gosd-init) is reported as "not found" rather than an error:
// callers move on to the next rung of the ladder.
func moduleRootForModule(dir string) (string, bool) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}}|{{.Dir}}")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}

	path, root, ok := strings.Cut(strings.TrimSpace(string(out)), "|")
	if !ok || path != GosdModulePath {
		return "", false
	}

	if info, err := os.Stat(filepath.Join(root, gosdInitRelPkg)); err != nil || !info.IsDir() {
		return "", false
	}
	return root, true
}

// moduleCacheDir implements rung 2: it resolves gosd's own build version via
// runtime/debug.ReadBuildInfo, then asks `go mod download` to fetch (or
// locate, if already cached) that exact version of gosd in the local module
// cache, returning its directory.
func moduleCacheDir() (string, error) {
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Path != GosdModulePath {
		return "", fmt.Errorf(
			"could not determine which version of %s built this gosd binary, so gosd-init's source can't be located; try passing --gosd-init-src <dir>",
			GosdModulePath)
	}

	version := bi.Main.Version
	if version == "" || version == "(devel)" {
		return "", fmt.Errorf(
			"this gosd binary was built from an unreleased/development copy of %s (version %q), so gosd-init's source can't be resolved from the module cache; "+
				"build gosd from a full checkout instead (`git clone %s && go install ./cmd/gosd` from inside it), or pass --gosd-init-src <dir> pointing at a checkout's %s directory",
			GosdModulePath, version, "https://"+GosdModulePath, gosdInitRelPkg)
	}

	query := fmt.Sprintf("%s@%s", GosdModulePath, version)
	cmd := exec.Command("go", "mod", "download", "-json", query)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf(
			"fetching %s to build gosd-init failed (this needs network access the first time a version is used; the module cache serves every build after that); try running `go mod download %s` directly to reproduce, or pass --gosd-init-src <dir>:\n%s",
			query, query, stderr.String())
	}

	var info struct {
		Dir   string
		Error string
	}
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return "", fmt.Errorf("parsing `go mod download -json %s` output failed: %w", query, err)
	}
	if info.Error != "" {
		return "", fmt.Errorf("resolving %s failed: %s; try passing --gosd-init-src <dir>", query, info.Error)
	}
	if info.Dir == "" {
		return "", fmt.Errorf("`go mod download -json %s` did not report a module directory; try passing --gosd-init-src <dir>", query)
	}
	return info.Dir, nil
}
