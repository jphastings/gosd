// Command injectfixture builds a small, real gosd image and its
// <image>.inject.json sidecar for js/packages/gosd's cross-implementation
// integration test: proof that the TypeScript client (parseManifest,
// padContents, createSubstitutionTransform) correctly consumes exactly what
// the Go side (internal/image.Write, internal/inject.Render/WriteManifest)
// actually produces, rather than a hand-rolled JS fixture that might
// silently drift from the real contract. It's covered by the repo's normal
// Go quality gates like every other package; js/packages/gosd's
// `npm run genfixture` script just runs it before the integration vitest
// project.
//
// Usage: go run ./internal/cmd/injectfixture <output-dir>
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/inject"
	"github.com/jphastings/gosd/internal/naming"
)

const bootSizeBytes = 8 * 1024 * 1024 // 8MiB - just enough for gosd.toml plus the two placeholders below.

// fixtureAppName stands in for the app a real `gosd build` would be given,
// so the fixture's partition labels are derived exactly as that build's
// would be (naming.LabelPrefix; see `gosd build --label-prefix`).
const fixtureAppName = "fixture"

// configReservedBytes matches `gosd build --config-placeholder`: the fixture
// carries a real reserved gosd.toml, rendered by the same code a real build
// uses, so the TypeScript client is proved against the actual bytes rather
// than a hand-written approximation of them.
const configReservedBytes = 8192

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

	placeholders := []inject.Placeholder{
		{Path: "config.yaml", SizeBytes: 4096},
		{Path: "net.cfg", SizeBytes: 2048},
	}

	gosdToml, err := gosdtoml.RenderReserved(
		fixtureAppName, true, "", "",
		gosdtoml.EnvSection{Values: map[string]string{"API_URL": "https://example.invalid"}},
		gosdtoml.Ingress{}, configReservedBytes,
	)
	if err != nil {
		return fmt.Errorf("rendering the fixture's gosd.toml failed: %w", err)
	}

	bootFiles := map[string]io.Reader{
		"gosd.toml": bytes.NewReader(gosdToml),
	}
	reportRanges := make([]string, 0, len(placeholders)+1)
	reportRanges = append(reportRanges, "gosd.toml")
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
		Board:               "test-fixture",
		Placeholders:        placeholders,
		ConfigReservedBytes: configReservedBytes,
		FileRanges:          report.FileRanges,
	})
	if err != nil {
		return fmt.Errorf("writing injection manifest for %s failed: %w", imgPath, err)
	}

	fmt.Println(imgPath)
	fmt.Println(manifestPath)
	return nil
}
