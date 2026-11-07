// Package nvmedata puts /data on an NVMe drive, for a board that keeps it
// there rather than in partition 2 of the media it boots from
// (initcfg.Config.DataOnNVMe; today only jetson-nano, whose eMMC NVIDIA's
// own partition layout owns).
//
// The formatting, growing and adopting are all internal/blockmount's, through
// the same marker-gated sequence the public disk and emmc packages use (see
// its establish doc for the crash-ordering argument). This package adds only
// what gosd-init needs on top: which drive, how long to wait for it, and
// advice for whoever is reading the serial console when it can't be used.
// It never passes blockmount's destructive flag, so the only drives it ever
// writes are blank ones and its own unestablished debris; and it only ever
// considers NVMe namespaces, so no other block device — the eMMC above all —
// can be chosen.
package nvmedata

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// ErrNoDrive reports that no usable NVMe drive is attached.
var ErrNoDrive = errors.New("no NVMe drive found")

// fitOne is the fix for ErrNoDrive.
const fitOne = "this board keeps /data on an NVMe drive, so fit one in the M.2 slot (a blank drive is formatted on first boot) and reboot"

// remedy ends every refusal to touch a drive that holds something else,
// replacing blockmount's destructive=true advice, which an operator has no
// way to give.
const remedy = "GoSD only formats a blank drive, so to use this one for /data, take it out and erase it on another computer (saving anything on it you need first), then reboot"

const (
	// pollEvery is how often awaitDrives looks again for a drive still
	// coming up.
	pollEvery = 250 * time.Millisecond
	// settle is how long awaitDrives waits after the first namespace
	// appears, for any siblings to appear too, so a choice between several
	// is never made from a partial list.
	settle = 500 * time.Millisecond
	// noControllerGrace is how long an enumerated PCIe bus may show no NVMe
	// controller before awaitDrives believes it: the kernel adds a root port
	// to sysfs before the device behind it, and a slow drive can still be
	// training its link.
	noControllerGrace = 3 * time.Second
)

// namespaceRE matches an NVMe namespace's block device (nvme0n1), and
// nothing else in /sys/block: not a partition, and not a multipath
// per-controller path (nvme0c0n1).
var namespaceRE = regexp.MustCompile(`^nvme\d+n\d+$`)

// PCIBus is what sysfs shows of the PCIe bus so far.
type PCIBus struct {
	// Functions counts the PCI functions enumerated.
	Functions int
	// NVMe counts the NVMe controllers among them.
	NVMe int
	// NVMeBound counts the NVMe controllers a driver has bound to.
	NVMeBound int
}

// Deps are the side-effecting operations Mount needs.
type Deps struct {
	// Storage is blockmount's format/grow/adopt machinery. Mount supplies
	// its Discover.
	Storage blockmount.Deps
	// Devices lists the block devices present, and the device nodes
	// currently mounted.
	Devices func() ([]blockmount.Device, map[string]bool, error)
	PCI     func() (PCIBus, error)
	Sleep   func(time.Duration)
	Log     func(format string, args ...any)
}

// Options are what to put on the drive, where, and how long to wait for it.
type Options struct {
	Filesystem diskfmt.FS
	Label      string
	Mountpoint string
	// Wait bounds how long to wait for an NVMe controller to appear.
	Wait time.Duration
	// NamespaceWait bounds how long to wait, once a controller is there,
	// for a namespace on it: a controller may take tens of seconds to
	// report ready.
	NamespaceWait time.Duration
}

// Mount mounts the NVMe drive's GoSD data volume at opts.Mountpoint,
// formatting the drive first only if it's blank (or holds an interrupted
// format of its own; see blockmount.Run). Anything else on the drive is
// refused, never erased.
func Mount(deps Deps, opts Options) error {
	drives, err := awaitDrives(deps, opts)
	if err != nil {
		return err
	}
	drive, err := choose(deps, drives, opts)
	if err != nil {
		return err
	}

	formatted := false
	storage := blockmount.Storage{Pkg: "gosd-init", Noun: "NVMe drive", Deps: deps.Storage, Remedy: remedy}
	storage.Deps.Discover = func() (string, error) { return drive, nil }
	format := storage.Deps.Format
	storage.Deps.Format = func(device, label string, fs diskfmt.FS) error {
		formatted = true
		deps.Log("formatting the NVMe drive at %s as %s, labelled %s", device, fs, label)
		return format(device, label, fs)
	}
	if _, err := blockmount.Run(storage, opts.Filesystem, opts.Label, opts.Mountpoint, false); err != nil {
		return err
	}

	if formatted {
		deps.Log("%s is a new %s volume on the NVMe drive at %s", opts.Mountpoint, opts.Filesystem, drive)
	} else {
		deps.Log("%s is the %s volume labelled %s already on the NVMe drive at %s", opts.Mountpoint, opts.Filesystem, opts.Label, drive)
	}
	return nil
}

// awaitDrives returns the NVMe namespaces present, waiting for one only
// while one can still appear: the PCIe bus may not have enumerated yet, and
// an NVMe controller's namespaces are scanned asynchronously after it
// probes. A bus that has stayed enumerated with no NVMe controller on it for
// noControllerGrace will never grow one, so that is reported without waiting
// out the rest.
//
// ponytail: assumes one PCIe host controller enumerates every slot (true of
// Tegra210); a board with several would wait for each.
func awaitDrives(deps Deps, opts Options) ([]string, error) {
	noControllerSince := time.Duration(-1)
	for waited := time.Duration(0); ; waited += pollEvery {
		drives, err := candidates(deps)
		if err != nil {
			return nil, err
		}
		if len(drives) > 0 {
			deps.Sleep(settle)
			if drives, err := candidates(deps); err != nil || len(drives) > 0 {
				return drives, err
			}
		}

		bus, err := deps.PCI()
		if err != nil {
			return nil, err
		}
		if bus.Functions > 0 && bus.NVMe == 0 {
			if noControllerSince < 0 {
				noControllerSince = waited
			}
		} else {
			noControllerSince = -1
		}

		switch {
		case noControllerSince >= 0 && waited-noControllerSince >= noControllerGrace:
			return nil, fmt.Errorf("%w: the PCIe bus has had no NVMe controller on it for %s; %s (a drive that's slow to come up can need another reboot)", ErrNoDrive, noControllerGrace, fitOne)
		case bus.Functions == 0 && waited >= opts.Wait:
			return nil, fmt.Errorf("%w: no PCIe device appeared within %s; %s", ErrNoDrive, opts.Wait, fitOne)
		case bus.NVMeBound == 0 && waited >= opts.NamespaceWait:
			return nil, fmt.Errorf("%w: an NVMe controller is on the PCIe bus, but no driver took it within %s; this board's kernel may lack CONFIG_BLK_DEV_NVME", ErrNoDrive, opts.NamespaceWait)
		case waited >= opts.NamespaceWait:
			return nil, fmt.Errorf("%w: the NVMe driver took the controller, but no writable namespace on it appeared within %s; check the drive in another computer", ErrNoDrive, opts.NamespaceWait)
		}
		deps.Sleep(pollEvery)
	}
}

// choose picks the namespace to use. With one, there's nothing to choose.
// With several, it never guesses: only a single namespace already carrying
// this app's volume is chosen, since that is the one /data has always been.
func choose(deps Deps, drives []string, opts Options) (string, error) {
	if len(drives) == 1 {
		return drives[0], nil
	}

	var ours []string
	for _, drive := range drives {
		contents, err := deps.Storage.Inspect(drive)
		if err != nil {
			return "", fmt.Errorf("inspecting the NVMe drive at %s failed: %w", drive, err)
		}
		if contents.FS == opts.Filesystem && strings.EqualFold(contents.Label, opts.Label) {
			ours = append(ours, drive)
		}
	}
	if len(ours) == 1 {
		deps.Log("found NVMe namespaces %s; /data is the one labelled %s, %s", strings.Join(drives, ", "), opts.Label, ours[0])
		return ours[0], nil
	}
	return "", fmt.Errorf("found NVMe namespaces %s, and %d of them carry this app's %s volume labelled %s, so GoSD won't guess which is /data; fit only the drive meant for /data (a blank one is formatted, and if several carry %s, erase all but one on another computer)",
		strings.Join(drives, ", "), len(ours), opts.Filesystem, opts.Label, opts.Label)
}

// candidates lists the usable NVMe namespaces, in name order.
func candidates(deps Deps) ([]string, error) {
	devices, mounted, err := deps.Devices()
	if err != nil {
		return nil, err
	}
	return blockmount.Candidates(devices, mounted, func(dev blockmount.Device) (int, bool) {
		return 0, namespaceRE.MatchString(dev.Name)
	}), nil
}
