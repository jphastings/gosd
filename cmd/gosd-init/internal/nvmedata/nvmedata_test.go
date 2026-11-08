package nvmedata

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

const label = "hello-data"

// drive is one block device as blockmount sees it.
type drive struct {
	contents    diskfmt.Contents
	marker      bool
	rootContent bool
	mountErr    error
	formats     int
}

func blank() *drive { return &drive{contents: diskfmt.Contents{Blank: true}} }

func ours(marker bool) *drive {
	return &drive{contents: diskfmt.Contents{FS: diskfmt.EXT4, Label: label}, marker: marker}
}

// look is what sysfs shows on one look: the /sys/block names, and the bus.
type look struct {
	names []string
	bus   PCIBus
}

// rig is a board whose sysfs shows looks[i] on the i-th look (the last one
// thereafter).
type rig struct {
	drives  map[string]*drive // by device node
	looks   []look
	seen    int
	now     look
	mounted *drive
	slept   time.Duration
}

func (r *rig) deps() Deps {
	return Deps{
		Storage: blockmount.Deps{
			MountedAt: func(string) (string, bool, error) { return "", false, nil },
			Inspect:   func(dev string) (diskfmt.Contents, error) { return r.drives[dev].contents, nil },
			Format: func(dev, label string, fs diskfmt.FS) error {
				*r.drives[dev] = drive{contents: diskfmt.Contents{FS: fs, Label: label}, formats: r.drives[dev].formats + 1}
				return nil
			},
			Mount: func(dev, _ string, _ diskfmt.FS) error {
				if err := r.drives[dev].mountErr; err != nil {
					return err
				}
				r.mounted = r.drives[dev]
				return nil
			},
			Mountable:           func(diskfmt.FS) (bool, error) { return true, nil },
			MountedSources:      func() (map[string]bool, error) { return map[string]bool{}, nil },
			SyncDevice:          func(string) error { return nil },
			Grow:                func(string, string) error { return nil },
			EstablishMarker:     func(string, diskfmt.FS) error { r.mounted.marker = true; return nil },
			MarkerEstablished:   func(string) (bool, error) { return r.mounted.marker, nil },
			RootHasOtherContent: func(string, diskfmt.FS) (bool, error) { return r.mounted.rootContent, nil },
			Unmount:             func(string) error { r.mounted = nil; return nil },
		},
		Devices: func() ([]blockmount.Device, map[string]bool, error) {
			r.now = r.looks[min(r.seen, len(r.looks)-1)]
			r.seen++
			var devices []blockmount.Device
			for _, name := range r.now.names {
				devices = append(devices, blockmount.Device{Name: name, SizeSectors: 1 << 30})
			}
			return devices, map[string]bool{}, nil
		},
		PCI:   func() (PCIBus, error) { return r.now.bus, nil },
		Sleep: func(d time.Duration) { r.slept += d },
		Log:   func(string, ...any) {},
	}
}

var nvmeBus = PCIBus{Functions: 2, NVMe: 1, NVMeBound: 1}

// oneDrive is a board with d as nvme0n1, there from the first look.
func oneDrive(d *drive) *rig {
	return &rig{drives: map[string]*drive{"/dev/nvme0n1": d}, looks: []look{{names: []string{"nvme0n1"}, bus: nvmeBus}}}
}

func mount(r *rig) error {
	return Mount(r.deps(), Options{Filesystem: diskfmt.EXT4, Label: label, Mountpoint: "/data", Wait: 10 * time.Second, NamespaceWait: 30 * time.Second})
}

func TestMountDecidesFromWhatTheDriveHolds(t *testing.T) {
	unmountable := ours(true)
	unmountable.mountErr = errors.New("bad superblock")
	debrisWithContent := ours(false)
	debrisWithContent.rootContent = true

	for name, tc := range map[string]struct {
		drive      *drive
		wantFormat bool
		wantErr    error
	}{
		"blank: formatted":                          {drive: blank(), wantFormat: true},
		"this app's volume: adopted":                {drive: ours(true)},
		"its interrupted first format: redone":      {drive: ours(false), wantFormat: true},
		"no marker but content: refused":            {drive: debrisWithContent, wantErr: blockmount.ErrRefusedFormat},
		"this app's volume, unmountable: refused":   {drive: unmountable, wantErr: blockmount.ErrUnmountable},
		"this app's label on FAT32: refused":        {drive: &drive{contents: diskfmt.Contents{FS: diskfmt.FAT32, Label: label}}, wantErr: blockmount.ErrRefusedFormat},
		"someone else's volume: refused":            {drive: &drive{contents: diskfmt.Contents{FS: diskfmt.FAT32, Label: "PHOTOS"}}, wantErr: blockmount.ErrRefusedFormat},
		"something GoSD can't read at all: refused": {drive: &drive{contents: diskfmt.Contents{OtherFS: "ntfs"}}, wantErr: blockmount.ErrRefusedFormat},
	} {
		t.Run(name, func(t *testing.T) {
			r := oneDrive(tc.drive)
			markerBefore := tc.drive.marker

			err := mount(r)
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil) != (err == nil) {
				t.Fatalf("Mount() = %v, want %v", err, tc.wantErr)
			}
			if formatted := tc.drive.formats > 0; formatted != tc.wantFormat {
				t.Errorf("formatted = %v, want %v", formatted, tc.wantFormat)
			}
			if tc.wantErr == nil {
				return
			}
			if tc.drive.marker != markerBefore {
				t.Error("a refused drive was written to")
			}
			if msg := err.Error(); !strings.Contains(msg, "erase it on another computer") || strings.Contains(msg, "destructive=true") {
				t.Errorf("Mount() = %q, want an operator's fix rather than the app API's destructive=true", msg)
			}
		})
	}
}

func TestMountOnlyEverChoosesAnNVMeNamespace(t *testing.T) {
	r := &rig{
		drives: map[string]*drive{"/dev/mmcblk0": blank(), "/dev/sda": blank(), "/dev/nvme0n1": blank()},
		looks:  []look{{names: []string{"mmcblk0", "mmcblk0boot0", "nvme0c0n1", "nvme0n1", "sda"}, bus: nvmeBus}},
	}

	if err := mount(r); err != nil {
		t.Fatalf("Mount() = %v, want nil", err)
	}
	if r.drives["/dev/nvme0n1"].formats != 1 || r.drives["/dev/mmcblk0"].formats+r.drives["/dev/sda"].formats != 0 {
		t.Error("formatted something other than exactly the NVMe namespace")
	}
}

func TestMountWithSeveralNamespacesNeverGuesses(t *testing.T) {
	twoDrives := func(first, second *drive) *rig {
		return &rig{
			drives: map[string]*drive{"/dev/nvme0n1": first, "/dev/nvme1n1": second},
			// The second appears a look after the first: settling must catch it.
			looks: []look{{names: []string{"nvme0n1"}, bus: nvmeBus}, {names: []string{"nvme0n1", "nvme1n1"}, bus: nvmeBus}},
		}
	}

	t.Run("the one carrying this app's volume is /data", func(t *testing.T) {
		r := twoDrives(blank(), ours(true))
		if err := mount(r); err != nil {
			t.Fatalf("Mount() = %v, want nil", err)
		}
		if r.mounted != r.drives["/dev/nvme1n1"] || r.drives["/dev/nvme0n1"].formats != 0 {
			t.Error("didn't adopt nvme1n1, the namespace carrying this app's volume, untouched")
		}
	})
	t.Run("two blank drives: refused", func(t *testing.T) {
		r := twoDrives(blank(), blank())
		if err := mount(r); err == nil {
			t.Fatal("Mount() = nil, want a refusal to guess")
		}
		if r.drives["/dev/nvme0n1"].formats+r.drives["/dev/nvme1n1"].formats != 0 {
			t.Error("formatted a drive while refusing to guess between them")
		}
	})
}

func TestMountWaitsForADriveStillComingUp(t *testing.T) {
	enumerated := PCIBus{Functions: 2}
	for name, looks := range map[string][]look{
		"the controller appears on the third look": {{bus: enumerated}, {bus: enumerated}, {bus: nvmeBus}, {names: []string{"nvme0n1"}, bus: nvmeBus}},
		"the namespace scan takes a while":         {{bus: nvmeBus}, {bus: nvmeBus}, {bus: nvmeBus}, {names: []string{"nvme0n1"}, bus: nvmeBus}},
	} {
		t.Run(name, func(t *testing.T) {
			r := &rig{drives: map[string]*drive{"/dev/nvme0n1": blank()}, looks: looks}
			if err := mount(r); err != nil {
				t.Fatalf("Mount() = %v, want nil once the drive appears", err)
			}
		})
	}
}

func TestMountGivesUpOnADriveThatCantAppear(t *testing.T) {
	for name, tc := range map[string]struct {
		bus       PCIBus
		wantSlept time.Duration
		wantMsg   string
	}{
		"the bus has no NVMe controller":         {bus: PCIBus{Functions: 3}, wantSlept: noControllerGrace, wantMsg: "no NVMe controller"},
		"the bus never enumerates":               {bus: PCIBus{}, wantSlept: 10 * time.Second, wantMsg: "no PCIe device"},
		"no driver takes the controller":         {bus: PCIBus{Functions: 2, NVMe: 1}, wantSlept: 30 * time.Second, wantMsg: "no driver"},
		"the controller never shows a namespace": {bus: nvmeBus, wantSlept: 30 * time.Second, wantMsg: "no writable namespace"},
	} {
		t.Run(name, func(t *testing.T) {
			r := &rig{looks: []look{{bus: tc.bus}}}

			err := mount(r)
			if !errors.Is(err, ErrNoDrive) || !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("Mount() = %v, want ErrNoDrive saying %q", err, tc.wantMsg)
			}
			if r.slept != tc.wantSlept {
				t.Errorf("waited %s, want %s", r.slept, tc.wantSlept)
			}
		})
	}
}
