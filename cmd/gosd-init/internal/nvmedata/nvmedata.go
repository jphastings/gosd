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

// pollEvery is how often awaitDrive looks again for a drive still coming up.
const pollEvery = 250 * time.Millisecond

// namespaceRE matches an NVMe namespace's block device (nvme0n1), and
// nothing else in /sys/block: not a partition, and not a multipath
// per-controller path (nvme0c0n1).
var namespaceRE = regexp.MustCompile(`^nvme\d+n\d+$`)

// Deps are the side-effecting operations Mount needs.
type Deps struct {
	// Storage is blockmount's format/grow/adopt machinery. Mount supplies
	// its Discover.
	Storage blockmount.Deps
	// Devices lists the block devices present, and the device nodes
	// currently mounted.
	Devices func() ([]blockmount.Device, map[string]bool, error)
	// PCI counts the PCI functions enumerated so far, and how many of them
	// are NVMe controllers.
	PCI   func() (functions, nvme int, err error)
	Sleep func(time.Duration)
	Log   func(format string, args ...any)
}

// Options are what to put on the drive, and where.
type Options struct {
	Filesystem diskfmt.FS
	Label      string
	Mountpoint string
	// Wait bounds how long to wait for a drive that can still appear.
	Wait time.Duration
}

// Mount mounts the NVMe drive's GoSD data volume at opts.Mountpoint,
// formatting the drive first only if it's blank (or holds an interrupted
// format of its own; see blockmount.Run). Anything else on the drive is
// refused, never erased.
func Mount(deps Deps, opts Options) error {
	drive, err := awaitDrive(deps, opts.Wait)
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

// awaitDrive returns the NVMe namespace to use, waiting up to wait for one
// only while one can still appear: an NVMe controller's namespaces are
// scanned asynchronously after it probes, and the PCIe bus itself may not
// have enumerated yet. A bus that has enumerated with no NVMe controller on
// it will never grow one, so that is reported at once.
//
// ponytail: with several namespaces it uses the lowest-named (nvme0n1, a
// drive's first namespace), since a Jetson Nano's carrier has one M.2 slot;
// a board with two NVMe controllers would need to choose by something
// stable across boots, like the volume label, since probe order isn't.
func awaitDrive(deps Deps, wait time.Duration) (string, error) {
	looksWithoutController := 0
	for waited := time.Duration(0); ; waited += pollEvery {
		devices, mounted, err := deps.Devices()
		if err != nil {
			return "", err
		}
		if drives := blockmount.Candidates(devices, mounted, rank); len(drives) > 0 {
			if len(drives) > 1 {
				deps.Log("found NVMe namespaces %v; keeping /data on %s", drives, drives[0])
			}
			return drives[0], nil
		}

		functions, controllers, err := deps.PCI()
		if err != nil {
			return "", err
		}
		// Two looks in a row, because the kernel adds an enumerated bus's
		// devices to sysfs one at a time: a single look can land between
		// a root port and the drive behind it.
		// ponytail: assumes one PCIe host controller enumerates every slot
		// (true of Tegra210); a board with several would wait for each.
		if functions > 0 && controllers == 0 {
			looksWithoutController++
		} else {
			looksWithoutController = 0
		}

		switch {
		case looksWithoutController >= 2:
			return "", fmt.Errorf("%w: the PCIe bus has no NVMe controller on it; %s", ErrNoDrive, fitOne)
		case waited >= wait && controllers > 0:
			return "", fmt.Errorf("%w: an NVMe controller is on the PCIe bus, but no writable namespace on it appeared within %s; check the drive in another computer", ErrNoDrive, wait)
		case waited >= wait:
			return "", fmt.Errorf("%w: no PCIe device appeared within %s; %s", ErrNoDrive, wait, fitOne)
		}
		deps.Sleep(pollEvery)
	}
}

func rank(dev blockmount.Device) (int, bool) {
	return 0, namespaceRE.MatchString(dev.Name)
}
