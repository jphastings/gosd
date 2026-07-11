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

// cacheInputs is exactly the locked cache key recipe (bean gosd-x488): the
// kernel ref, container image digest, GoSD fragment + patches, developer
// overlay, and output filenames. Marshaled to JSON (struct field order is
// deterministic) and hashed - anything not listed here (e.g. RequiredY,
// KBUILD_* pins) intentionally does not affect the cache key.
type cacheInputs struct {
	Repo            string
	Ref             string
	Image           string
	Fragment        []byte
	Patches         []cachePatch
	OverlayFragment []byte
	OverlayPatches  []cachePatch
	OutputNames     []string
}

// cacheKey computes the content-addressed cache key for building spec (with
// overlay) inside image.
func cacheKey(spec kernelspec.KernelSpec, overlay Overlay, image string) (string, error) {
	in := cacheInputs{
		Repo:            spec.Source.Repo,
		Ref:             spec.Source.Ref,
		Image:           image,
		Fragment:        spec.ConfigFragment,
		OverlayFragment: overlay.ConfigFragment,
		OutputNames:     outputFilenames(spec),
	}
	for _, p := range spec.DTSPatches {
		in.Patches = append(in.Patches, cachePatch{Name: p.Name, Content: p.Content})
	}
	for _, p := range overlay.Patches {
		in.OverlayPatches = append(in.OverlayPatches, cachePatch{Name: p.Name, Content: p.Content})
	}

	data, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("kernelbuild: hashing cache inputs: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// outputFilenames is the board's own artifact output names (the kernel
// image, and its DTB if any) - the "output names" cache key input, and the
// set of files an artifacts-dir consumer looks for.
func outputFilenames(spec kernelspec.KernelSpec) []string {
	names := []string{spec.KernelFilename}
	if spec.DTB != nil {
		names = append(names, spec.DTB.Filename)
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
