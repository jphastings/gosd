package kernelbuild

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/kernelspec"
)

// cachePatch is the hashed shape of a kernelspec.Patch: name matters (it's
// part of apply order/identity) as well as content.
type cachePatch struct {
	Name    string
	Content []byte
}

// cacheDTB is the hashed shape of a kernelspec.DTB: MakeTarget and
// SourcePath both feed the generated build script (see buildScript in
// script.go), so either changing silently re-serves a stale cached DTB if
// omitted. Filename is not repeated here - it's already covered by
// OutputNames.
type cacheDTB struct {
	MakeTarget string
	SourcePath string
}

// cacheInputs is the cache key recipe. It started as bean gosd-x488's locked
// recipe - kernel ref, container image digest, GoSD fragment + patches,
// developer overlay, and output filenames - but bean gosd-7jmj found that
// recipe incomplete in practice: buildScript (script.go) also bakes
// Defconfig, Toolchain (ARCH/CROSS_COMPILE/cross package),
// KernelMakeTarget, KernelSourcePath, and each DTB's MakeTarget/SourcePath
// into the generated build script, none of which were in the key, so fixing
// any of them alone silently re-served the old, wrong cached kernel/DTB
// forever. This struct is the superseding, complete recipe - it supersedes
// gosd-x488's field list for that reason. Marshaled to JSON (struct field
// order is deterministic) and hashed.
//
// What's still intentionally excluded, and why: RequiredY, ForbiddenY and
// ModulesDisabled are gates checked AFTER the config is built, not inputs
// that change what gets built, so a fix to one of those assertions doesn't
// need to invalidate an otherwise-identical cached kernel (pinned by
// TestBuild_CacheHitsDespiteRequiredYChange); the Reproducibility/KBUILD_*
// pins are passed as container environment variables, never baked into the
// script itself, so they don't affect the built output either.
type cacheInputs struct {
	Repo             string
	Ref              string
	Image            string
	Defconfig        string
	ToolchainArch    string
	ToolchainCross   string
	Fragment         []byte
	Patches          []cachePatch
	OverlayFragment  []byte
	OverlayPatches   []cachePatch
	KernelMakeTarget string
	KernelSourcePath string
	DTBs             []cacheDTB
	OutputNames      []string
}

// cacheKey computes the content-addressed cache key for building spec (with
// overlay) inside image.
func cacheKey(spec kernelspec.KernelSpec, overlay Overlay, image string) (string, error) {
	in := cacheInputs{
		Repo:             spec.Source.Repo,
		Ref:              spec.Source.Ref,
		Image:            image,
		Defconfig:        spec.Defconfig,
		ToolchainArch:    spec.Toolchain.KernelArch,
		ToolchainCross:   spec.Toolchain.CrossCompile,
		Fragment:         spec.ConfigFragment,
		OverlayFragment:  overlay.ConfigFragment,
		KernelMakeTarget: spec.KernelMakeTarget,
		KernelSourcePath: spec.KernelSourcePath,
		OutputNames:      outputFilenames(spec),
	}
	for _, p := range spec.DTSPatches {
		in.Patches = append(in.Patches, cachePatch{Name: p.Name, Content: p.Content})
	}
	for _, p := range overlay.Patches {
		in.OverlayPatches = append(in.OverlayPatches, cachePatch{Name: p.Name, Content: p.Content})
	}
	for _, dtb := range spec.AllDTBs() {
		in.DTBs = append(in.DTBs, cacheDTB{MakeTarget: dtb.MakeTarget, SourcePath: dtb.SourcePath})
	}

	data, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("kernelbuild: hashing cache inputs: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// outputFilenames is the board's own artifact output names (the kernel
// image, and its DTBs if any) - the "output names" cache key input, and the
// set of files an artifacts-dir consumer looks for.
func outputFilenames(spec kernelspec.KernelSpec) []string {
	names := []string{spec.KernelFilename}
	for _, dtb := range spec.AllDTBs() {
		names = append(names, dtb.Filename)
	}
	return names
}

// allOutputNames is outputFilenames plus the two files every build also
// produces: the generated .config and source.json.
func allOutputNames(spec kernelspec.KernelSpec) []string {
	return append(outputFilenames(spec), generatedConfigName, sourceJSONName)
}

// cacheComplete reports whether dir already holds every file spec's build
// is expected to produce, i.e. whether Build can skip running the
// container entirely.
func cacheComplete(dir string, spec kernelspec.KernelSpec) bool {
	for _, name := range allOutputNames(spec) {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}
