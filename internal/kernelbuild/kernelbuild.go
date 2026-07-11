// Package kernelbuild builds a board's kernel inside a container from its
// kernelspec.KernelSpec plus an optional developer Overlay, emitting a flat
// artifact directory and/or the staging/<board> layout
// build/artifacts/package.sh consumes - output that drops straight into
// `gosd build --artifacts-dir`. See bean gosd-x488 and epic gosd-47rm.
//
// Builds are content-addressed and cached under
// os.UserCacheDir()/gosd/kernel-build/<key>/ (key = hash of the kernel ref,
// container image digest, GoSD fragment/patches, developer overlay, and
// output filenames - see cacheKey): a call whose key already has every
// expected output present skips the container run entirely and reports
// that via Result.Skipped.
package kernelbuild

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/kernelspec"
)

// runner is the subset of *container.Runtime this package needs, defined
// locally (rather than as an exported interface in internal/container) so
// tests can inject a fake that "succeeds" by writing expected output files
// without a real container runtime. *container.Runtime satisfies it as-is.
type runner interface {
	Run(ctx context.Context, spec container.RunSpec) error
}

// Options configures a Build call.
type Options struct {
	// Runtime runs the build container - normally a *container.Runtime
	// from container.Detect. Required.
	Runtime runner
	// Image overrides the container image the build runs inside. Empty
	// uses container.KernelBuildImage.
	Image string
	// CacheDir overrides the cache root (normally
	// os.UserCacheDir()/gosd/kernel-build). Tests supply a temp dir here;
	// production callers normally leave it empty.
	CacheDir string
	// Outputs selects where a successful (or cache-hit) build's artifacts
	// are written.
	Outputs Outputs
	// Stdout and Stderr receive the container's build output live, since
	// kernel builds run 20-60 minutes. Either may be nil to discard that
	// stream.
	Stdout, Stderr io.Writer
}

// Result reports what Build did.
type Result struct {
	// CacheKey is the content-addressed cache key this build resolved to.
	CacheKey string
	// Skipped is true when a matching cache entry already had every
	// expected output present, so no container was run.
	Skipped bool
	// CacheDir is where this build's canonical outputs live.
	CacheDir string
}

// Build builds spec's kernel, with overlay layered on top (its zero value
// is a no-op), inside opts.Runtime, and writes the result to
// opts.Outputs.
func Build(ctx context.Context, spec kernelspec.KernelSpec, overlay Overlay, opts Options) (Result, error) {
	if opts.Runtime == nil {
		return Result{}, fmt.Errorf("kernelbuild: Options.Runtime is required")
	}

	image := opts.Image
	if image == "" {
		image = container.KernelBuildImage
	}

	cacheRoot, err := resolveCacheRoot(opts.CacheDir)
	if err != nil {
		return Result{}, err
	}

	key, err := cacheKey(spec, overlay, image)
	if err != nil {
		return Result{}, err
	}
	entryDir := filepath.Join(cacheRoot, key)

	if cacheComplete(entryDir, spec) {
		if err := collectOutputs(entryDir, spec, opts.Outputs); err != nil {
			return Result{}, err
		}
		return Result{CacheKey: key, Skipped: true, CacheDir: entryDir}, nil
	}

	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return Result{}, fmt.Errorf("kernelbuild: creating cache dir %s: %w", cacheRoot, err)
	}

	if err := runBuild(ctx, spec, overlay, image, cacheRoot, entryDir, opts); err != nil {
		return Result{}, err
	}

	if err := collectOutputs(entryDir, spec, opts.Outputs); err != nil {
		return Result{}, err
	}
	return Result{CacheKey: key, Skipped: false, CacheDir: entryDir}, nil
}

func resolveCacheRoot(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("kernelbuild: resolving user cache dir: %w", err)
	}
	return filepath.Join(base, "gosd", "kernel-build"), nil
}

// runBuild runs one container build and, on success, moves its outputs into
// the cache atomically: it builds into a temp directory under cacheRoot and
// os.Renames it to entryDir only once every expected output is present, so
// an interrupted or failed build never leaves a half-written cache entry
// (same pattern as internal/artifacts.ensureBoard).
func runBuild(ctx context.Context, spec kernelspec.KernelSpec, overlay Overlay, image, cacheRoot, entryDir string, opts Options) error {
	// The work dir must live under cacheRoot, not os.TempDir(): on macOS the
	// default temp dir is /var/folders/…, which Docker Desktop's VM does not
	// share by default, so a bind mount from there appears empty inside the
	// container. cacheRoot sits under the user's home, which is shared.
	workDir, err := os.MkdirTemp(cacheRoot, "work-*")
	if err != nil {
		return fmt.Errorf("kernelbuild: creating work dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	if err := writeWorkDir(workDir, spec, overlay); err != nil {
		return fmt.Errorf("kernelbuild: preparing build script: %w", err)
	}

	tmpOut, err := os.MkdirTemp(cacheRoot, "build.tmp-*")
	if err != nil {
		return fmt.Errorf("kernelbuild: creating output dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpOut) }() // no-op once the rename below succeeds

	runSpec := container.RunSpec{
		Image: image,
		Env:   buildEnv(spec),
		Mounts: []container.Mount{
			{HostPath: workDir, ContainerPath: "/work", ReadOnly: true},
			{HostPath: tmpOut, ContainerPath: "/out", ReadOnly: false},
		},
		Cmd:    []string{"bash", "/work/" + workScriptName},
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	}
	if err := opts.Runtime.Run(ctx, runSpec); err != nil {
		return fmt.Errorf("kernel build for %s failed: %w", spec.BoardID, err)
	}

	if err := verifyBuildOutputs(tmpOut, spec); err != nil {
		return err
	}
	if err := writeSourceJSON(tmpOut, spec); err != nil {
		return err
	}

	if err := os.RemoveAll(entryDir); err != nil {
		return fmt.Errorf("kernelbuild: clearing stale cache entry %s: %w", entryDir, err)
	}
	if err := os.Rename(tmpOut, entryDir); err != nil {
		return fmt.Errorf("kernelbuild: moving build outputs into cache at %s: %w", entryDir, err)
	}
	return nil
}

// buildEnv is the container environment every `make` invocation in the
// generated script relies on: the ARCH/CROSS_COMPILE toolchain pair, plus
// whichever KBUILD_BUILD_* reproducibility pins spec sets (see
// kernelspec.Reproducibility - empty for boards that don't pin them yet).
func buildEnv(spec kernelspec.KernelSpec) map[string]string {
	env := map[string]string{
		"ARCH":          spec.Toolchain.KernelArch,
		"CROSS_COMPILE": spec.Toolchain.CrossCompile,
	}
	if v := spec.Reproducibility.KBUILDBuildTimestamp; v != "" {
		env["KBUILD_BUILD_TIMESTAMP"] = v
	}
	if v := spec.Reproducibility.KBUILDBuildUser; v != "" {
		env["KBUILD_BUILD_USER"] = v
	}
	if v := spec.Reproducibility.KBUILDBuildHost; v != "" {
		env["KBUILD_BUILD_HOST"] = v
	}
	return env
}

// verifyBuildOutputs confirms the container actually produced everything
// the script's final "copy outputs" step promises, before this build is
// trusted enough to move into the cache.
func verifyBuildOutputs(dir string, spec kernelspec.KernelSpec) error {
	for _, name := range append(outputFilenames(spec), generatedConfigName) {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("kernel build for %s did not produce expected output %q: %w", spec.BoardID, name, err)
		}
	}
	return nil
}
