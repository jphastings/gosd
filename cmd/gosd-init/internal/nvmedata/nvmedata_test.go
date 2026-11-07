package nvmedata

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

const label = "hello-data"

// fakeDrive is one NVMe drive as blockmount sees it, and every format it
// takes.
type fakeDrive struct {
	contents diskfmt.Contents
	marker   bool
	formats  []string
}

// rig is a board whose sysfs shows looks[i] on the i-th look (the last one
// thereafter), with pci reporting its PCIe bus.
type rig struct {
	drive          fakeDrive
	looks          [][]blockmount.Device
	pciFunctions   int
	pciControllers int
	slept          time.Duration
	logs           []string
}

func (r *rig) deps() Deps {
	look := 0
	return Deps{
		Storage: blockmount.Deps{
			MountedAt: func(string) (string, bool, error) { return "", false, nil },
			Inspect:   func(string) (diskfmt.Contents, error) { return r.drive.contents, nil },
			Format: func(device, label string, fs diskfmt.FS) error {
				r.drive.formats = append(r.drive.formats, device)
				r.drive.contents = diskfmt.Contents{FS: fs, Label: label}
				return nil
			},
			Mount:               func(string, string, diskfmt.FS) error { return nil },
			Mountable:           func(diskfmt.FS) (bool, error) { return true, nil },
			MountedSources:      func() (map[string]bool, error) { return map[string]bool{}, nil },
			SyncDevice:          func(string) error { return nil },
			Grow:                func(string, string) error { return nil },
			EstablishMarker:     func(string, diskfmt.FS) error { r.drive.marker = true; return nil },
			MarkerEstablished:   func(string) (bool, error) { return r.drive.marker, nil },
			RootHasOtherContent: func(string, diskfmt.FS) (bool, error) { return false, nil },
			Unmount:             func(string) error { return nil },
		},
		Devices: func() ([]blockmount.Device, map[string]bool, error) {
			devices := r.looks[min(look, len(r.looks)-1)]
			look++
			return devices, map[string]bool{}, nil
		},
		PCI:   func() (int, int, error) { return r.pciFunctions, r.pciControllers, nil },
		Sleep: func(d time.Duration) { r.slept += d },
		Log:   func(format string, args ...any) { r.logs = append(r.logs, fmt.Sprintf(format, args...)) },
	}
}

func dev(name string) blockmount.Device { return blockmount.Device{Name: name, SizeSectors: 1 << 30} }

func mount(r *rig) error {
	return Mount(r.deps(), Options{Filesystem: diskfmt.EXT4, Label: label, Mountpoint: "/data", Wait: 2 * time.Second})
}

func TestMountFormatsABlankDrive(t *testing.T) {
	r := &rig{drive: fakeDrive{contents: diskfmt.Contents{Blank: true}}, looks: [][]blockmount.Device{{dev("nvme0n1")}}}

	if err := mount(r); err != nil {
		t.Fatalf("Mount() = %v, want nil", err)
	}
	if len(r.drive.formats) != 1 || r.drive.formats[0] != "/dev/nvme0n1" || !r.drive.marker {
		t.Errorf("formats = %v, marker = %v; want /dev/nvme0n1 formatted once and established", r.drive.formats, r.drive.marker)
	}
}

func TestMountAdoptsAnEstablishedVolume(t *testing.T) {
	r := &rig{
		drive: fakeDrive{contents: diskfmt.Contents{FS: diskfmt.EXT4, Label: label}, marker: true},
		looks: [][]blockmount.Device{{dev("nvme0n1")}},
	}

	if err := mount(r); err != nil {
		t.Fatalf("Mount() = %v, want nil", err)
	}
	if len(r.drive.formats) != 0 {
		t.Errorf("formatted %v, want the established volume adopted untouched", r.drive.formats)
	}
}

func TestMountRefusesADriveHoldingSomethingElse(t *testing.T) {
	r := &rig{
		drive: fakeDrive{contents: diskfmt.Contents{FS: diskfmt.FAT32, Label: "PHOTOS"}},
		looks: [][]blockmount.Device{{dev("nvme0n1")}},
	}

	err := mount(r)
	if !errors.Is(err, blockmount.ErrRefusedFormat) {
		t.Fatalf("Mount() = %v, want ErrRefusedFormat", err)
	}
	if len(r.drive.formats) != 0 {
		t.Errorf("formatted %v, want a drive holding something else left alone", r.drive.formats)
	}
	if msg := err.Error(); !strings.Contains(msg, "erase it on another computer") || strings.Contains(msg, "destructive=true") {
		t.Errorf("Mount() = %q, want an operator's fix rather than the app API's destructive=true", msg)
	}
}

func TestMountFailsFastWhenThePCIeBusHasNoNVMeController(t *testing.T) {
	// The eMMC, its hardware partitions and a USB stick are all present and
	// usable, and none of them may ever become /data.
	notNVMe := []blockmount.Device{dev("mmcblk0"), dev("mmcblk0boot0"), dev("sda"), dev("nvme0c0n1")}
	r := &rig{looks: [][]blockmount.Device{notNVMe}, pciFunctions: 3}

	if err := mount(r); !errors.Is(err, ErrNoDrive) {
		t.Fatalf("Mount() = %v, want ErrNoDrive", err)
	}
	if r.slept > pollEvery {
		t.Errorf("waited %s for a drive the PCIe bus shows can't appear, want one confirming look", r.slept)
	}
}

func TestMountWaitsForANamespaceStillBeingScanned(t *testing.T) {
	r := &rig{
		drive:          fakeDrive{contents: diskfmt.Contents{Blank: true}},
		looks:          [][]blockmount.Device{{}, {}, {}, {dev("nvme0n1")}},
		pciFunctions:   2,
		pciControllers: 1,
	}

	if err := mount(r); err != nil {
		t.Fatalf("Mount() = %v, want nil once the namespace appears", err)
	}
	if r.slept != 3*pollEvery {
		t.Errorf("waited %s, want exactly until the namespace appeared (%s)", r.slept, 3*pollEvery)
	}
}

func TestMountGivesUpOnADriveThatNeverAppears(t *testing.T) {
	for name, controllers := range map[string]int{"bus never enumerates": 0, "namespace never appears": 1} {
		t.Run(name, func(t *testing.T) {
			r := &rig{looks: [][]blockmount.Device{{}}, pciFunctions: controllers, pciControllers: controllers}

			if err := mount(r); !errors.Is(err, ErrNoDrive) {
				t.Fatalf("Mount() = %v, want ErrNoDrive", err)
			}
			if r.slept != 2*time.Second {
				t.Errorf("waited %s, want the whole 2s bound", r.slept)
			}
		})
	}
}
