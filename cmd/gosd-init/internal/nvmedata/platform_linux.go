//go:build linux

package nvmedata

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// NewDeps wires the real sysfs reads and blockmount's Linux operations, the
// same ones the public disk package wires.
func NewDeps(log func(format string, args ...any)) Deps {
	return Deps{
		Storage: blockmount.Deps{
			MountedAt:           blockmount.MountedAt,
			Inspect:             diskfmt.Inspect,
			Format:              diskfmt.Format,
			Mount:               blockmount.Mount,
			Mountable:           blockmount.Mountable,
			MountedSources:      blockmount.MountedSources,
			SyncDevice:          blockmount.SyncDevice,
			Grow:                blockmount.GrowEXT4,
			EstablishMarker:     blockmount.EstablishMarker,
			MarkerEstablished:   blockmount.MarkerEstablished,
			RootHasOtherContent: blockmount.RootHasOtherContent,
			Unmount:             blockmount.Unmount,
		},
		Devices: devices,
		PCI:     pci,
		Sleep:   time.Sleep,
		Log:     log,
	}
}

func devices() ([]blockmount.Device, map[string]bool, error) {
	devs, err := blockmount.ReadBlockDevices()
	if err != nil {
		return nil, nil, err
	}
	mounted, err := blockmount.MountedSources()
	if err != nil {
		return nil, nil, err
	}
	return devs, mounted, nil
}

const pciDevicesDir = "/sys/bus/pci/devices"

// nvmeClass is the PCI class code of an NVMe controller: mass storage (01),
// non-volatile memory (08), NVMe programming interface (02).
const nvmeClass = "0x010802"

// pci counts the PCI functions in sysfs and the NVMe controllers among them.
// A function whose class can't be read isn't counted at all, so it can never
// be the reason awaitDrive decides no drive is coming.
func pci() (functions, nvme int, err error) {
	entries, err := os.ReadDir(pciDevicesDir)
	if os.IsNotExist(err) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		class, err := os.ReadFile(filepath.Join(pciDevicesDir, e.Name(), "class"))
		if err != nil {
			continue
		}
		functions++
		if strings.TrimSpace(string(class)) == nvmeClass {
			nvme++
		}
	}
	return functions, nvme, nil
}
