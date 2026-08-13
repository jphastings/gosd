// Command injectfixture builds a small, real gosd image and its
// <image>.inject.json sidecar for js/packages/gosd's cross-implementation
// integration test: proof that the TypeScript client (parseManifest,
// padContents, createSubstitutionTransform) correctly consumes exactly what
// the Go side (internal/configtree, internal/image.Write,
// internal/inject.Render/WriteManifest) actually produces, rather than a
// hand-rolled JS fixture that might silently drift from the real contract.
// It's covered by the repo's normal Go quality gates like every other
// package; js/packages/gosd's `npm run genfixture` script just runs it
// before the integration vitest project.
//
// Usage: go run ./internal/cmd/injectfixture <output-dir>
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/configtree"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/inject"
	"github.com/jphastings/gosd/internal/naming"
)

// bootSizeBytes is enough for the two placeholders below and a full config
// tree - each of whose files costs a FAT cluster of its own.
const bootSizeBytes = 16 * 1024 * 1024

// fixtureAppName stands in for the app a real `gosd build` would be given,
// so the fixture's partition labels are derived exactly as that build's
// would be (naming.LabelPrefix; see `gosd build --label-prefix`).
const fixtureAppName = "fixture"

// fixtureEnvName is the app-supplied setting the fixture's --config-dir
// overlay adds, so the TypeScript integration test can inject an
// env/<NAME> value into a tree gosd really built.
const fixtureEnvName = "API_TOKEN"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "injectfixture: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: injectfixture <output-dir>")
	}
	outDir := args[0]

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory %s failed: %w", outDir, err)
	}

	overlay, err := writeOverlay()
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(overlay) }()

	// Cloudflared's settings are switched on so the fixture carries a
	// feature-pruned directory too, exactly as `gosd build --ingress
	// cloudflared` produces one.
	tree, err := configtree.Build(overlay, configtree.Features{IngressCloudflared: true})
	if err != nil {
		return fmt.Errorf("building the fixture's config tree failed: %w", err)
	}

	placeholders := []inject.Placeholder{
		{Path: "config.yaml", SizeBytes: 4096},
		{Path: "net.cfg", SizeBytes: 2048},
	}

	bootFiles := make(map[string]io.Reader)
	reportRanges := make([]string, 0, len(placeholders)+len(tree.Values))
	for path, content := range tree.BootFiles() {
		bootFiles[path] = bytes.NewReader(content)
	}
	for _, v := range tree.Values {
		reportRanges = append(reportRanges, configtree.Dir+"/"+v.Path)
	}
	for _, p := range placeholders {
		rendered, err := inject.Render(p)
		if err != nil {
			return fmt.Errorf("rendering placeholder %q failed: %w", p.Path, err)
		}
		bootFiles[p.Path] = bytes.NewReader(rendered)
		reportRanges = append(reportRanges, p.Path)
	}

	imgPath := filepath.Join(outDir, "fixture.img")
	// image.Write refuses to overwrite an existing file, but re-running
	// `npm run genfixture` against a leftover fixture from a previous run
	// (the fixture directory is gitignored, not cleaned between runs) is
	// the normal case during development, not an error - so clear the way
	// first.
	if err := os.Remove(imgPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing previous fixture image %s failed: %w", imgPath, err)
	}
	labels := naming.LabelsFor(naming.LabelPrefix(fixtureAppName))
	report, err := image.Write(imgPath, image.Spec{
		BootLabel:     labels.Boot,
		DataLabel:     labels.Data,
		BootSizeBytes: bootSizeBytes,
		BootFiles:     bootFiles,
		ReportRanges:  reportRanges,
	})
	if err != nil {
		return fmt.Errorf("writing fixture image %s failed: %w", imgPath, err)
	}

	manifestPath, err := inject.WriteManifest(imgPath, inject.ManifestSpec{
		Board:        "test-fixture",
		Placeholders: placeholders,
		Config:       tree,
		FileRanges:   report.FileRanges,
	})
	if err != nil {
		return fmt.Errorf("writing injection manifest for %s failed: %w", imgPath, err)
	}

	fmt.Println(imgPath)
	fmt.Println(manifestPath)
	return nil
}

// writeOverlay creates the app-side --config-dir the fixture builds with:
// one app-owned environment variable, documented as gosd's build gate
// requires. The caller removes the directory.
func writeOverlay() (string, error) {
	dir, err := os.MkdirTemp("", "injectfixture-config-")
	if err != nil {
		return "", fmt.Errorf("creating the fixture's config overlay failed: %w", err)
	}

	envDir := filepath.Join(dir, "env")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		return "", fmt.Errorf("creating the fixture's config overlay failed: %w", err)
	}
	files := map[string][]byte{
		fixtureEnvName:                        nil,
		fixtureEnvName + configtree.DocSuffix: []byte("# " + fixtureEnvName + "\n\nThe token the fixture app talks to its server with.\n"),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(envDir, name), content, 0o644); err != nil {
			return "", fmt.Errorf("writing the fixture's config overlay failed: %w", err)
		}
	}
	return dir, nil
}
