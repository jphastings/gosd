// Package boards is the registry of SD-card targets gosd knows how to build
// for, and the Board abstraction each target implements. Boards target
// per-board architectures (see Arch) - most are arm64, but some (the Pi Zero
// W, GOARM=6) are 32-bit; adding a board means implementing Board in its own
// sub-package (see pizero2w and radxazero3e) and registering it (see
// cmd/gosd).
package boards

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

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
	// UsbGadget is set when --usb-gadget was passed: the board's USB
	// controller should boot in peripheral mode so an app using the
	// gadget package has a UDC to bind to. Boards that need a boot-time
	// change for this (Pi Zero 2W's dwc2 overlay) read it here; boards
	// that don't (Radxa Zero 3E's dwc3 negotiates role automatically)
	// ignore it.
	UsbGadget bool

	// Env holds developer-set default app environment variables (gosd
	// build --env, repeatable KEY=VALUE). The build pipeline bakes these
	// into both /etc/gosd/config.json (initcfg.Config.Env) and the
	// rendered gosd.toml [env] section - see gosd-9b5c's locked
	// precedence (a hand-edited gosd.toml [env] entry overrides the same
	// key here).
	Env map[string]string
}

// GadgetSupport reports whether a board can boot its USB controller into
// peripheral mode at the pinned kernel and device tree its Artifacts resolve
// to - the fact `gosd build --usb-gadget` checks, board by board, before
// assembling any image (see COMPATIBILITY.md's USB gadget row). This is a
// static fact about the pinned artifacts, not a boot-time choice: it doesn't
// change based on BuildConfig.UsbGadget, and it's independent of whether a
// capable board also needs a boot-file change (BootFiles reads
// BuildConfig.UsbGadget for that).
type GadgetSupport struct {
	// Supported is false when the board's pinned kernel/DT has no USB
	// peripheral controller for the gadget package to bind to.
	Supported bool
	// Reason explains why when Supported is false: it's folded verbatim
	// into gosd build's --usb-gadget error, so it should name the missing
	// hardware/DT node and, if there's a bean tracking the fix, mention
	// it. Unused (and may be empty) when Supported is true.
	Reason string
}

// Arch is the Go cross-compile target a board's binaries (the user's app and
// gosd-init) need: GOOS is always "linux" (internal/build hard-codes it), so
// only GOARCH and, for architectures that need it (arm), GOARM vary per
// board.
type Arch struct {
	// GOARCH is the target architecture, e.g. "arm64" or "arm".
	GOARCH string
	// GOARM is the target ARM architecture version, e.g. "6". Empty for
	// architectures where GOARM doesn't apply (arm64 and anything other
	// than GOARCH=arm).
	GOARM string
}

// KnownArches is the fixed vocabulary of target Arch values gosd's board
// fleet uses today (see CLAUDE.md's Target locked decision), keyed by
// Arch.Key(). It's independent of which boards happen to be registered: a
// package that needs to validate an arch token from developer input (e.g.
// internal/extconfig's gosd-external.toml arch = [...] list) can't rely on
// the live registry, since boards only register themselves from cmd/gosd's
// init() - a standalone parser test never runs that. Adding a board whose
// Arch() isn't already listed here needs a new entry alongside it.
var KnownArches = map[string]Arch{
	"arm64": {GOARCH: "arm64"},
	"arm-6": {GOARCH: "arm", GOARM: "6"},
}

// Key returns a short, filesystem- and map-safe identifier for a - the
// distinct value internal/build's per-arch compile cache and cmd/gosd's
// dedupe logic key on, e.g. "arm64" or "arm-6".
func (a Arch) Key() string {
	if a.GOARM == "" {
		return a.GOARCH
	}
	return a.GOARCH + "-" + a.GOARM
}

// Board is a single supported hardware target: naming, the artifacts it
// needs fetched/cached, and how to turn those artifacts into a bootable
// image's contents.
type Board interface {
	// Name is the stable, user-facing identifier used on the --board
	// flag and in output filenames (e.g. "pi-zero-2w").
	Name() string

	// Arch is the GOARCH/GOARM this board's binaries must be
	// cross-compiled for (GOOS is always linux). Two boards that return
	// the same Arch share one compile pass (see cmd/gosd's build
	// pipeline): it's compared by value, not by board identity.
	Arch() Arch

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

	// UsbGadgetSupport reports whether this board can boot its USB
	// controller into peripheral mode at the pinned artifacts. gosd
	// build --usb-gadget consults this for every selected board before
	// compiling or assembling anything, and refuses to build for any
	// board that returns Supported: false, rather than producing an
	// image whose app can never find a UDC at /sys/class/udc.
	UsbGadgetSupport() GadgetSupport
}

// BuildTag returns the Go build tag gosd passes to the app compile (and only
// the app compile - gosd-init is never tagged) so a developer can gate
// board-specific source with `//go:build gosd_<id>`. The `gosd_` prefix
// keeps the result a valid build-tag identifier even for ids starting with a
// digit; the id's hyphens (illegal in a build tag) are replaced with
// underscores, e.g. "pi-zero-2w" becomes "gosd_pi_zero_2w".
func BuildTag(b Board) string {
	return "gosd_" + strings.ReplaceAll(b.Name(), "-", "_")
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
