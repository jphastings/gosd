package boards

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/fetch"
)

// Artifacts holds every artifact a Board asked for in Artifacts(), already
// resolved to local files by ResolveArtifacts, plus the initramfs archive
// the build pipeline assembles before calling BootFiles.
type Artifacts struct {
	paths map[string]string

	// Initramfs is the already-built initramfs archive. It is nil while
	// FirmwareFiles and RawWrites run (the initramfs embeds
	// FirmwareFiles' output, so it can't exist yet), and set by the time
	// BootFiles is called.
	Initramfs io.Reader
}

// Open returns a freshly opened reader for the artifact named name. Callers
// (typically a Board's BootFiles) are responsible for closing it; the build
// pipeline closes every reader a Board hands back once it's done with them.
func (a Artifacts) Open(name string) (io.ReadCloser, error) {
	path, ok := a.paths[name]
	if !ok {
		return nil, fmt.Errorf("artifact %q was not resolved (want one of the board's declared Artifacts())", name)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening artifact %q at %s: %w", name, path, err)
	}
	return f, nil
}

// MustOpen is like Open, but panics instead of returning an error. It exists
// for FirmwareFiles and RawWrites, whose interface signatures can't return
// an error: a failure there means a Board asked for a name it never
// declared in Artifacts(), which is a programmer error, not a runtime one.
func (a Artifacts) MustOpen(name string) io.Reader {
	r, err := a.Open(name)
	if err != nil {
		panic(fmt.Sprintf("boards: %v", err))
	}
	return r
}

// ResolveArtifacts turns refs into an Artifacts value for board: each ref is
// looked up by name inside artifactsDir first, then (if it has one) fetched
// by its pinned URL (verified against ref.SHA256, cached in cacheDir), then
// — for a ref with no URL — resolved via fetchBoardArtifacts, which lazily
// downloads and caches board's whole CI-built artifact release into
// cacheDir at most once per call to ResolveArtifacts, however many such refs
// there are. A ref with no URL that isn't found in artifactsDir, when
// fetchBoardArtifacts is nil or itself fails, is reported as an actionable
// error.
func ResolveArtifacts(ctx context.Context, board string, refs []ArtifactRef, artifactsDir, cacheDir string, fetchBoardArtifacts BoardArtifactsFunc) (Artifacts, error) {
	paths := make(map[string]string, len(refs))

	var (
		boardArtifactsDir      string
		boardArtifactsErr      error
		boardArtifactsResolved bool
	)
	ensureBoardArtifacts := func() (string, error) {
		if !boardArtifactsResolved {
			boardArtifactsDir, boardArtifactsErr = fetchBoardArtifacts(ctx, cacheDir, board)
			boardArtifactsResolved = true
		}
		return boardArtifactsDir, boardArtifactsErr
	}

	for _, ref := range refs {
		if artifactsDir != "" {
			local := filepath.Join(artifactsDir, ref.Name)
			if _, err := os.Stat(local); err == nil {
				paths[ref.Name] = local
				continue
			}
		}

		if ref.URL != "" {
			path, err := fetch.ToDir(ctx, nil, fetch.File{URL: ref.URL, SHA256: ref.SHA256}, cacheDir, ref.Name)
			if err != nil {
				return Artifacts{}, fmt.Errorf("fetching artifact %q: %w", ref.Name, err)
			}
			paths[ref.Name] = path
			continue
		}

		if fetchBoardArtifacts == nil {
			return Artifacts{}, fmt.Errorf(
				"artifact %q was not found in --artifacts-dir %q, and no CI-built artifact release source is "+
					"configured; supply it via --artifacts-dir", ref.Name, artifactsDir)
		}

		dir, err := ensureBoardArtifacts()
		if err != nil {
			return Artifacts{}, fmt.Errorf(
				"artifact %q was not found in --artifacts-dir %q, and downloading %s's CI-built artifact "+
					"release failed: %w; supply it via --artifacts-dir instead", ref.Name, artifactsDir, board, err)
		}
		local := filepath.Join(dir, ref.Name)
		if _, err := os.Stat(local); err != nil {
			return Artifacts{}, fmt.Errorf(
				"artifact %q was not found in --artifacts-dir %q nor in %s's downloaded CI-built artifact "+
					"release; check the release manifest, or supply it via --artifacts-dir", ref.Name, artifactsDir, board)
		}
		paths[ref.Name] = local
	}

	return Artifacts{paths: paths}, nil
}
