// Package boards is the registry of SD-card targets gosd knows how to build
// for, and the Board abstraction each target implements. Every board GoSD
// supports is arm64; adding a board means implementing Board in its own
// sub-package (see pizero2w and radxazero3e) and registering it (see
// cmd/gosd).
package boards

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/jphastings/gosd/internal/image"
)

// ArtifactRef pins one file a Board needs to assemble its image: a kernel,
// device tree blob, firmware, or bootloader file. Board.Artifacts lists
// these; ResolveArtifacts turns the list into local files, preferring an
// --artifacts-dir override, then a pinned-URL fetch, then (for refs with no
// URL) the CI-built artifact release fetched via a BoardArtifactsFunc.
type ArtifactRef struct {
	// Name identifies the artifact; it is both the file name expected
	// inside --artifacts-dir and the cache key used when fetching URL.
	Name string
	// URL is the upstream location to fetch Name from when it isn't found
	// in --artifacts-dir. Empty means Name isn't fetched by its own
	// pinned URL — either because it's a third-party blob with no
	// per-file source yet, or (more commonly) because it's one of the
	// kernel/DTB/bootloader files GoSD compiles itself and ships as part
	// of a whole per-board artifact release instead (see
	// BoardArtifactsFunc).
	URL string
	// SHA256 is the expected digest of the fetched file, required
	// whenever URL is set.
	SHA256 string
}

// BoardArtifactsFunc downloads and caches every CI-built artifact (kernel,
// DTB, bootloader — whatever a board doesn't fetch via a per-file pinned
// URL) for the given board under cacheDir, and returns the local directory
// they were extracted into. internal/artifacts.EnsureBoard implements this
// signature; ResolveArtifacts calls it, when non-nil, as the fallback for
// any ArtifactRef with no URL that isn't found in --artifacts-dir.
type BoardArtifactsFunc func(ctx context.Context, cacheDir, board string) (string, error)

// BuildConfig holds the per-build values a Board's BootFiles may need to
// bake into rendered templates (most boards only need these inside
// /etc/gosd/config.json, which the build pipeline writes itself, but the
// interface passes BuildConfig through in case a board's boot-time template
// needs them directly).
type BuildConfig struct {
	Hostname     string
	WifiSSID     string
	WifiPassword string
}

// Board is a single supported hardware target: naming, the artifacts it
// needs fetched/cached, and how to turn those artifacts into a bootable
// image's contents.
type Board interface {
	// Name is the stable, user-facing identifier used on the --board
	// flag and in output filenames (e.g. "pi-zero-2w").
	Name() string

	// Artifacts lists every kernel, DTB, firmware, and bootloader file
	// this board needs. The build pipeline resolves each ref (via
	// ResolveArtifacts) before calling BootFiles, RawWrites, or
	// FirmwareFiles.
	Artifacts() []ArtifactRef

	// BootFiles returns the FAT boot partition's contents, keyed by their
	// path inside that partition: the kernel, the initramfs (available at
	// the well-known initramfs name via art's resolved artifacts - see
	// Artifacts), and board-specific text files such as config.txt/
	// cmdline.txt or extlinux/extlinux.conf.
	BootFiles(cfg BuildConfig, art Artifacts) (map[string]io.Reader, error)

	// RawWrites returns any raw byte writes into the unpartitioned gap
	// ahead of the boot partition (e.g. a Rockchip bootloader). Empty for
	// boards with no such bootloader.
	RawWrites(art Artifacts) []image.RawWrite

	// FirmwareFiles returns the files that land under /lib/firmware/**
	// inside the initramfs, keyed by their path relative to
	// /lib/firmware. Empty for boards with no runtime-loaded firmware.
	FirmwareFiles(art Artifacts) map[string]io.Reader
}

var registry = map[string]Board{}

// internalOnly marks the subset of registry that RegisterInternal (rather
// than Register) added: boards that exist and are fully buildable, but are
// deliberately left out of every user-facing listing. qemu-virt is the only
// current member (see that package's doc comment) - it's for CI/local
// testing, not something an end user should be offered as a flashing
// target.
var internalOnly = map[string]bool{}

// Register adds b to the set of known boards, keyed by b.Name(), and
// includes it in All(), IDs(), and catalog generation. It's meant to be
// called once at startup for every public board implementation (see
// cmd/gosd); registering the same name twice (via Register or
// RegisterInternal) is a programmer error.
func Register(b Board) {
	register(b, false)
}

// RegisterInternal adds b to the set of known boards, keyed by b.Name(),
// exactly like Register, except All(), IDs(), and catalog generation all
// skip it - the board is only reachable via an explicit --board=<name>
// (Find still finds it). Use this for boards that are real and fully
// buildable but not meant to be offered to end users, e.g. qemu-virt.
func RegisterInternal(b Board) {
	register(b, true)
}

func register(b Board, internal bool) {
	name := b.Name()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("boards: %q is already registered", name))
	}
	registry[name] = b
	if internal {
		internalOnly[name] = true
	}
}

// All returns every registered public board, sorted by name - i.e. every
// board except those registered via RegisterInternal. This is the default
// no---board build set and the set catalog generation draws from.
func All() []Board {
	out := make([]Board, 0, len(registry))
	for name, b := range registry {
		if internalOnly[name] {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Find looks up a board by its ID (Name()), public or internal-only: an
// explicit --board=qemu-virt must still resolve even though qemu-virt is
// absent from All()/IDs().
func Find(id string) (Board, bool) {
	b, ok := registry[id]
	return b, ok
}

// IDs returns the IDs of every registered public board, sorted - the same
// set All() returns, for --help text and error messages that shouldn't
// advertise internal-only boards.
func IDs() []string {
	ids := make([]string, 0, len(registry))
	for name := range registry {
		if internalOnly[name] {
			continue
		}
		ids = append(ids, name)
	}
	sort.Strings(ids)
	return ids
}

// IsInternal reports whether id refers to a board registered via
// RegisterInternal. Callers that resolve boards explicitly (e.g. --board)
// and then need to exclude internal-only ones from a public-facing output -
// catalog generation is the current example - use this rather than
// re-deriving the distinction themselves.
func IsInternal(id string) bool {
	return internalOnly[id]
}
