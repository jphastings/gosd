// Package extbuild builds one "external" companion binary (e.g. a static
// mpv) inside a container from a developer-authored build script (see
// internal/extconfig for the gosd-external.toml parser that produces one),
// mirroring how internal/kernelbuild works: a content-addressed, durable
// on-disk cache, built via internal/container, with its output verified
// afterwards as a fully static ELF binary matching the target arch
// (internal/staticelf) and GPL-style provenance recorded to source.json.
//
// internal/extbuild is a SIBLING of internal/kernelbuild, not a
// parameterization of it: kernelbuild generates the entire build script
// itself from a declarative kernelspec.KernelSpec, while extbuild's script
// is an opaque, developer-owned artifact it only wraps. Sharing an
// abstraction across those two shapes would only obscure both. See bean
// gosd-sn30 and epic gosd-oyhi.
package extbuild

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/staticelf"
)

// runner mirrors kernelbuild's local runner seam: only the method this
// package needs from *container.Runtime, defined locally so tests can
// inject a fake that "succeeds" by writing the expected output file
// without a real container runtime. *container.Runtime satisfies it as-is.
type runner interface {
	Run(ctx context.Context, spec container.RunSpec) error
}

// Source is one provenance entry recorded to source.json: what upstream
// repo/ref/license the build script pins and clones. GoSD itself never
// clones or verifies these - they are provenance-recording only (the GPL
// carve-out locked in epic gosd-oyhi).
type Source struct {
	Name    string
	Repo    string
	Ref     string
	License string
}

// Spec is one external binary build: a developer's script cross-compiling
// for a single target arch. A recipe declaring multiple arches (see
// extconfig.External.Arch) means one Spec - and one Build call - per arch.
type Spec struct {
	// Name identifies the external (e.g. "mpv"): the output filename
	// inside /out (GOSD_OUTPUT=/out/<Name>) and a cache-key input.
	Name string
	// Script is the developer's build script contents, mounted read-only
	// at /work and run inside the container by a small generated wrapper
	// (see writeWorkDir).
	Script []byte
	// Arch is the target GOARCH/GOARM the script must cross-compile for.
	Arch boards.Arch
	// Sources is provenance recorded to source.json; may be empty.
	Sources []Source
}

// Options configures a Build call.
type Options struct {
	// Runtime runs the build container - normally a *container.Runtime
	// from container.Detect. Required.
	Runtime runner
	// Image overrides the container image the build runs inside. Empty
	// uses container.KernelBuildImage - the SAME base image gosd
	// build-kernel uses, so Docker's layer cache stays warm across both
	// (an explicit ask in epic gosd-oyhi).
	Image string
	// CacheDir overrides the cache root (normally the per-OS durable state
	// dir from defaultBuildRoot - deliberately not os.UserCacheDir, which
	// macOS may purge mid-build, mirroring kernelbuild). Tests supply a
	// temp dir here; production callers normally leave it empty.
	CacheDir string
	// OutputDir, if non-empty, receives a copy of the built binary (named
	// Spec.Name) and source.json once the build succeeds (or hits cache).
	OutputDir string
	// Stdout and Stderr receive the container's build output live. Either
	// may be nil to discard that stream.
	Stdout, Stderr io.Writer
}

// Result reports what Build did.
type Result struct {
	// CacheKey is the content-addressed cache key this build resolved to.
	CacheKey string
	// Skipped is true when a matching cache entry already had the
	// expected output present, so no container was run.
	Skipped bool
	// CacheDir is where this build's canonical outputs live.
	CacheDir string
	// OutputPath is the path a caller should open/read the built binary
	// from: the copy in Options.OutputDir if one was requested, otherwise
	// the cache entry's own copy.
	OutputPath string
}

// Build builds spec inside opts.Runtime, verifies its output, and writes
// the result to opts.OutputDir (when set).
func Build(ctx context.Context, spec Spec, opts Options) (Result, error) {
	if opts.Runtime == nil {
		return Result{}, fmt.Errorf("extbuild: Options.Runtime is required")
	}
	if spec.Name == "" {
		return Result{}, fmt.Errorf("extbuild: Spec.Name is required")
	}
	if strings.ContainsAny(spec.Name, "/\\") {
		return Result{}, fmt.Errorf("extbuild: Spec.Name %q must be a single path component (no slashes)", spec.Name)
	}
	if len(spec.Script) == 0 {
		return Result{}, fmt.Errorf("extbuild: Spec.Script is required")
	}
	crossCompile, err := crossCompilePrefix(spec.Arch)
	if err != nil {
		return Result{}, err
	}

	image := opts.Image
	if image == "" {
		image = container.KernelBuildImage
	}

	cacheRoot, err := resolveCacheRoot(opts.CacheDir)
	if err != nil {
		return Result{}, err
	}

	key, err := cacheKey(spec, image)
	if err != nil {
		return Result{}, err
	}
	entryDir := filepath.Join(cacheRoot, key)

	if cacheComplete(entryDir, spec.Name) {
		outPath, err := collectOutput(entryDir, spec.Name, opts.OutputDir)
		if err != nil {
			return Result{}, err
		}
		return Result{CacheKey: key, Skipped: true, CacheDir: entryDir, OutputPath: outPath}, nil
	}

	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return Result{}, fmt.Errorf("extbuild: creating cache dir %s: %w", cacheRoot, err)
	}

	if err := runBuild(ctx, spec, crossCompile, image, cacheRoot, entryDir, opts); err != nil {
		return Result{}, err
	}

	outPath, err := collectOutput(entryDir, spec.Name, opts.OutputDir)
	if err != nil {
		return Result{}, err
	}
	return Result{CacheKey: key, Skipped: false, CacheDir: entryDir, OutputPath: outPath}, nil
}

func resolveCacheRoot(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	root, err := defaultBuildRoot()
	if err != nil {
		return "", fmt.Errorf("extbuild: resolving build root: %w", err)
	}
	return root, nil
}

// runBuild runs one container build and, on success, moves its outputs into
// the cache atomically: it builds into a temp directory under cacheRoot and
// os.Renames it to entryDir only once the expected output has passed
// verification, so an interrupted or failed build never leaves a
// half-written cache entry (same pattern as kernelbuild.runBuild).
func runBuild(ctx context.Context, spec Spec, crossCompile, image, cacheRoot, entryDir string, opts Options) error {
	// The work dir must live under cacheRoot, not os.TempDir(): on macOS the
	// default temp dir is /var/folders/…, which the container VMs (colima
	// mounts $HOME and /tmp/colima; Docker Desktop shares a configured list)
	// don't reliably share, so a bind mount from there appears empty inside
	// the container (see kernelbuild.runBuild, gosd-0p21).
	workDir, err := os.MkdirTemp(cacheRoot, "work-*")
	if err != nil {
		return fmt.Errorf("extbuild: creating work dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	if err := writeWorkDir(workDir, spec); err != nil {
		return fmt.Errorf("extbuild: preparing build script: %w", err)
	}

	tmpOut, err := os.MkdirTemp(cacheRoot, "build.tmp-*")
	if err != nil {
		return fmt.Errorf("extbuild: creating output dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpOut) }() // no-op once the rename below succeeds

	runSpec := container.RunSpec{
		Image: image,
		Env:   buildEnv(spec, crossCompile),
		Mounts: []container.Mount{
			{HostPath: workDir, ContainerPath: "/work", ReadOnly: true},
			{HostPath: tmpOut, ContainerPath: "/out", ReadOnly: false},
		},
		Cmd:    []string{"bash", "/work/" + workWrapperName},
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	}
	if err := opts.Runtime.Run(ctx, runSpec); err != nil {
		if _, statErr := os.Stat(tmpOut); os.IsNotExist(statErr) {
			return vanishedStagingError(tmpOut, err)
		}
		return fmt.Errorf("external build for %s failed: %w", spec.Name, err)
	}
	if _, err := os.Stat(tmpOut); os.IsNotExist(err) {
		return vanishedStagingError(tmpOut, nil)
	}

	outPath := filepath.Join(tmpOut, spec.Name)
	if err := verifyOutput(outPath, spec.Arch); err != nil {
		return err
	}
	if err := writeSourceJSON(tmpOut, spec); err != nil {
		return err
	}

	if err := os.RemoveAll(entryDir); err != nil {
		return fmt.Errorf("extbuild: clearing stale cache entry %s: %w", entryDir, err)
	}
	if err := os.Rename(tmpOut, entryDir); err != nil {
		return fmt.Errorf("extbuild: moving build outputs into cache at %s: %w", entryDir, err)
	}
	return nil
}

// buildEnv is the container contract's env vars (bean gosd-sn30's locked
// decision): GOSD_ARCH (the recipe's own arch token, e.g. "arm64" or
// "arm-6" - unambiguous, unlike bare GOARCH=arm), GOSD_CROSS_COMPILE (the
// CROSS_COMPILE-style toolchain prefix, e.g. "aarch64-linux-gnu-"), and
// GOSD_OUTPUT (the exact path the script must write its binary to).
func buildEnv(spec Spec, crossCompile string) map[string]string {
	return map[string]string{
		"GOSD_ARCH":          spec.Arch.Key(),
		"GOSD_CROSS_COMPILE": crossCompile,
		"GOSD_OUTPUT":        "/out/" + spec.Name,
	}
}

// crossCompilePrefix maps arch to the CROSS_COMPILE-style toolchain prefix
// gosd's build-kernel image installs a cross-compiler under, mirroring the
// same two-entry vocabulary kernelspec's per-board Toolchain.CrossCompile
// values use. It errors for any GOARCH gosd doesn't yet know how to map,
// rather than silently handing a script an empty prefix.
func crossCompilePrefix(arch boards.Arch) (string, error) {
	switch arch.GOARCH {
	case "arm64":
		return "aarch64-linux-gnu-", nil
	case "arm":
		return "arm-linux-gnueabihf-", nil
	default:
		return "", fmt.Errorf("extbuild: no CROSS_COMPILE prefix known for GOARCH=%s", arch.GOARCH)
	}
}

// verifyOutput confirms path exists and is a fully static ELF binary
// matching arch (bean gosd-sn30's locked post-run verification): the
// initramfs has no ld.so or library layout, so a dynamically linked or
// wrong-arch output fails the build outright rather than reaching the
// cache.
func verifyOutput(path string, arch boards.Arch) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("external build did not produce the expected output %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if err := staticelf.Verify(f, path, arch); err != nil {
		return fmt.Errorf("external build output %s failed verification: %w", path, err)
	}
	return nil
}
