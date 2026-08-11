// Package emmc lets a GoSD app use the onboard eMMC storage on boards that
// have it (the Rockchip boards — Radxa Zero 3E, NanoPi Zero2, ROCK 4SE),
// formatting it on first use and mounting it on every subsequent boot.
//
// Unlike the microSD card the board boots from, the eMMC is soldered on (or,
// on the ROCK 4SE, an optional plug-in module) and ships blank, so it cannot
// be formatted on another machine. FormatAndMount therefore formats it in
// place — a whole-device filesystem, no partition table — and is idempotent
// across runs: once an eMMC carries a volume with the app's chosen label
// under the requested filesystem, later runs only mount it.
//
// ext4 is the default (Options.Filesystem's zero value, EXT4 — the same
// deliberate breaking change disk made, gosd-9sc4, ship notes in the release
// that carries it): it is journaled and crash-safe, which FAT32 and exFAT
// are neither, and the onboard eMMC is exactly the kind of internal drive
// where that matters. FAT32 and exFAT remain available as explicit
// Options.Filesystem choices for the case ext4 does not serve — removable
// media meant to be read on another host, where FAT32's universal
// readability (or exFAT's, past FAT32's 4 GiB-per-file/256 GiB-volume
// ceilings) is the point, not a limitation, though an onboard eMMC is rarely
// removable in practice. Whichever filesystem is in play, none of the three
// gives unix permissions or symlinks, and none but ext4's journal buys crash
// consistency for anything but its own metadata — write application data
// with the temp-file-then-rename pattern as for GOSD_DATA (see
// docs/runtime.md's fsync pattern).
//
// An eMMC already carrying a volume with the app's chosen label is mounted,
// not reformatted — but only when its filesystem also matches what was
// asked for. A label match against a *different* filesystem (an eMMC
// formatted by an app built before this default existed, or by a previous
// FormatAndMountWith(Options{Filesystem: ...}) choice) is treated like any
// other foreign content: see destructive, below.
//
// Formatting is destructive, so it is gated: FormatAndMount will format a blank
// eMMC freely, but refuses to overwrite anything else (a volume with a
// different label, or a filesystem GoSD cannot read) unless the caller
// explicitly opts in, returning an error wrapping ErrRefusedFormat
// otherwise.
//
// Unlike the sibling disk package — which discovers among several candidate
// mass-storage devices and can also be aimed at one by name
// (FormatAndMountDevice/Devices) — emmc addresses exactly one device, the
// board's own onboard eMMC (see chooseEMMC), so there is no emmc equivalent
// of those two calls. That candidate-selection difference is unrelated to,
// and unchanged by, which filesystem is requested — see
// internal/blockmount's package doc for the full split.
//
// For any other mass storage — an NVMe SSD, a USB drive, an SD card in a reader
// — see the sibling disk package, which has the same shape.
package emmc

import (
	"errors"
	"fmt"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// ErrNoEMMC reports that the board has no onboard eMMC available to format and
// mount — either it has none at all (e.g. a Raspberry Pi board), or the only
// eMMC present is the device the board booted from and so is off-limits.
var ErrNoEMMC = errors.New("no onboard eMMC found")

// ErrRefusedFormat reports that the eMMC already holds other content — a
// volume with a different label, or a filesystem GoSD cannot read — and
// destructive was false, so FormatAndMount left it untouched instead of wiping
// it. Callers that want to offer the user a way to consent (e.g. an app-env var
// read from gosd.toml's [env] table) can match this with errors.Is and retry
// with destructive=true once they have it.
var ErrRefusedFormat = blockmount.ErrRefusedFormat

// ErrUnsupportedFS reports that the board's kernel cannot mount the filesystem
// the work needed — either one the caller asked for, or the one the eMMC
// already carries. Not every board's kernel has ext4 or exFAT; this is
// reported before anything is written, so a caller can match it with
// errors.Is and fall back to FAT32 knowing the eMMC is untouched.
var ErrUnsupportedFS = blockmount.ErrUnsupportedFS

// ErrUnmountable reports the narrower case within ErrRefusedFormat: the eMMC
// already carries a volume labelled and formatted exactly as asked for, but
// it could not be mounted. Unlike ErrRefusedFormat's other causes, this is
// never someone else's data — it is the eMMC's own volume, unhealthy. The
// mount-failure refusal wraps both sentinels, so it never replaces
// ErrRefusedFormat and an existing errors.Is(err, ErrRefusedFormat) caller
// sees no change. An app that exposes one config flag authorizing adoption of
// a drive that isn't its own yet should gate on a second, differently-named
// flag before retrying with destructive=true on ErrUnmountable — that retry
// destroys the app's own data, not a stranger's.
var ErrUnmountable = blockmount.ErrUnmountable

// FormatAndMount ensures the board's onboard eMMC carries an ext4 filesystem
// labelled label and mounts it read-write at mountpoint, then reports the
// outcome on the returned channel.
//
// It returns immediately; the work runs in the background so the caller can
// continue starting up. The channel receives exactly one Result and is then
// closed. A typical caller blocks on it only when it first needs the storage:
//
//	res := <-emmc.FormatAndMount("APPDATA", "/storage", false)
//	if res.Err != nil {
//		log.Printf("no persistent storage: %v", res.Err)
//	}
//	// res.MountPoint is ready to use; res.BlockDevice is the node behind it.
//
// The eMMC is discovered automatically — see chooseEMMC for exactly which
// block device qualifies. An eMMC already carrying an ext4 volume with this
// label is only mounted, never reformatted (nor re-grown — growth happens
// exactly once, when the volume is first established), which is how re-runs
// of the same app avoid wiping their own data. An eMMC carrying a *different*
// filesystem under this label — e.g. one formatted by an app built before
// this default existed, or by a previous
// FormatAndMountWith(Options{Filesystem: ...}) choice — is treated as other
// content: see destructive, below. A blank eMMC (no filesystem and an
// all-zero leading region) is always formatted.
//
// destructive governs only an eMMC that already holds *other* data — content
// under a different label, or the same label under a different filesystem:
// false makes FormatAndMount fail without touching it, wrapping
// ErrRefusedFormat; true wipes and reformats it. label is limited to 16
// ASCII characters (ext4's limit; FormatAndMountWith with Options{Filesystem:
// FAT32} or ExFAT caps at 11 instead).
func FormatAndMount(label, mountpoint string, destructive bool) <-chan Result {
	return FormatAndMountWith(label, mountpoint, Options{Destructive: destructive})
}

// Filesystem names an on-disk filesystem FormatAndMountWith can create on the
// eMMC. It mirrors disk.Filesystem exactly, token for token — see that
// package's doc for what each one costs and buys.
type Filesystem string

const (
	// EXT4 is the default (Options.Filesystem's zero value) — a deliberate
	// breaking change from emmc's earlier FAT32-only default (gosd-9sc4,
	// mirroring disk's own flip, gosd-lfu0/gosd-1c0x), called out in the
	// release notes of the version that shipped it. It is journaled and
	// crash-safe (see docs/runtime.md for what the journal does and does
	// not buy — metadata consistency and mount-time replay, not data
	// durability), at the cost of not being natively readable from macOS
	// or Windows and needing CONFIG_EXT4_FS in the board's kernel — see
	// COMPATIBILITY.md for which boards have it (the Rockchip fleet does;
	// the Pi family has no onboard eMMC at all). Asking for it on a board
	// whose kernel lacks it fails with ErrUnsupportedFS before the eMMC is
	// touched. Formatting writes GoSD's checked-in golden ext4 image (see
	// internal/diskfmt), then grows it to the eMMC's actual size exactly
	// once, at establishment.
	EXT4 Filesystem = "ext4"
	// FAT32 is universal — every host mounts it, and every GoSD board's
	// kernel can — at the price of a hard 4 GiB ceiling on any single file,
	// however large the eMMC, a 256 GiB ceiling on the volume GoSD will
	// create (formatting a bigger eMMC fails with an error naming the
	// limit, leaving the eMMC untouched), and no crash safety. It remains
	// available for the case ext4's default does not serve.
	FAT32 Filesystem = "fat32"
	// ExFAT lifts FAT32's two ceilings while staying widely host-readable,
	// at the cost of needing exFAT support in the board's kernel — see
	// COMPATIBILITY.md for which boards have it — and, like FAT32, no crash
	// safety. Asking for it on a board that lacks it fails with
	// ErrUnsupportedFS before the eMMC is touched.
	ExFAT Filesystem = "exfat"
)

// Options are the choices FormatAndMountWith makes available. Its zero value
// formats ext4, discovers the eMMC, and refuses to overwrite anything
// already there.
type Options struct {
	// Filesystem is what to format the eMMC as if formatting is needed. The
	// zero value is EXT4. It has no say over an eMMC that already carries a
	// volume with the app's label AND this filesystem: that is mounted, not
	// reformatted. A label match against a *different* filesystem is not
	// silently converted — see Destructive.
	Filesystem Filesystem
	// Destructive allows overwriting an eMMC that holds other content —
	// including a volume under the app's label whose filesystem does not
	// match Filesystem. The zero value refuses, wrapping ErrRefusedFormat.
	// It has no bearing on a blank eMMC, which is always formatted.
	Destructive bool
}

// FormatAndMountWith is FormatAndMount with the choices spelled out — the
// filesystem to create and whether other content may be overwritten:
//
//	res := <-emmc.FormatAndMountWith("APPDATA", "/storage", emmc.Options{
//		Filesystem:  emmc.ExFAT,
//		Destructive: true,
//	})
//
// Everything else matches FormatAndMount, including returning immediately and
// delivering exactly one Result before closing the channel. Unlike disk's
// Options, there is no Device field: emmc always addresses the board's one
// onboard eMMC.
func FormatAndMountWith(label, mountpoint string, opts Options) <-chan Result {
	out := make(chan Result, 1)
	go func() {
		defer close(out)

		fs, err := opts.filesystem()
		if err != nil {
			out <- Result{Err: err}
			return
		}

		device, err := blockmount.Run(storage(newPlatformDeps()), fs, label, mountpoint, opts.Destructive)
		if err != nil {
			out <- Result{Err: err}
			return
		}
		out <- Result{MountPoint: mountpoint, BlockDevice: device}
	}()
	return out
}

func (o Options) filesystem() (diskfmt.FS, error) {
	switch o.Filesystem {
	case "", EXT4:
		return diskfmt.EXT4, nil
	case FAT32:
		return diskfmt.FAT32, nil
	case ExFAT:
		return diskfmt.ExFAT, nil
	default:
		return "", fmt.Errorf("emmc: %q is not a filesystem GoSD can create; use emmc.EXT4, emmc.FAT32 or emmc.ExFAT", string(o.Filesystem))
	}
}

// Result is the outcome of a FormatAndMount, delivered once on its channel. On
// success Err is nil and MountPoint/BlockDevice name the ready filesystem and
// the device behind it; on failure Err explains why and the other fields are
// empty.
type Result struct {
	// MountPoint is where the eMMC's filesystem is mounted read-write — the
	// mountpoint passed to FormatAndMount.
	MountPoint string
	// BlockDevice is the device node backing MountPoint, e.g. "/dev/mmcblk0".
	// The eMMC carries a whole-device filesystem (no partition table), so
	// this whole-device node can be handed straight to gadget.MassStorage to
	// share over USB — but Unmount MountPoint first: expose the device or mount
	// it, never both at once.
	BlockDevice string
	// Err is non-nil if the eMMC could not be formatted and mounted, including
	// ErrNoEMMC when the board has none and ErrRefusedFormat when it already
	// holds other content and destructive was false.
	Err error
}

// storage describes the onboard eMMC to the shared orchestration.
func storage(d blockmount.Deps) blockmount.Storage {
	return blockmount.Storage{Pkg: "emmc", Noun: "eMMC", Deps: d}
}

// chooseEMMC picks the onboard eMMC from the block devices present. It selects
// the eMMC (device/type "MMC", which distinguishes soldered eMMC from the "SD"
// card, independent of mmcblk numbering) that the board is not currently
// running from — a device with any mounted partition is the boot device and is
// never a format target. mountedSources holds the device nodes currently
// mounted (e.g. "/dev/mmcblk1p1"), so booting from the eMMC safely yields
// ErrNoEMMC rather than a wiped system.
//
// Unlike disk.rank (see gosd-f226), this has no explicit exclusion for an
// eMMC's hardware partitions (boot0/boot1/rpmb/gp0-gp3). It does not need one
// today only because of a sysfs-topology quirk: those gendisks' device/type
// attribute reads empty, not "MMC", so Kind == "MMC" already excludes them.
// That quirk is the only thing standing between this rank and a hardware
// partition; present-medium and write-protected exclusion no longer depend
// on any such quirk — blockmount.Usable enforces both structurally, for
// every caller of Choose/Candidates, so this rank need only express eMMC's
// own class preference (see gosd-ix38, which found disk enforcing
// SizeSectors/ReadOnly and this rank not). This selection logic is
// independent of which Filesystem is requested (gosd-9sc4 changed the
// latter, not the former): chooseEMMC never widens to disk's multi-class
// ranking, and there remains no emmc equivalent of disk's named-device
// selection.
func chooseEMMC(devices []blockmount.Device, mountedSources map[string]bool) (string, error) {
	rank := func(dev blockmount.Device) (int, bool) { return 0, dev.Kind == "MMC" }
	return blockmount.Choose(devices, mountedSources, rank, ErrNoEMMC)
}
