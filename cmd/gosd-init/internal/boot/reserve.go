package boot

import "github.com/jphastings/gosd/internal/devreserve"

// bootPartitionRole is the prose a refusal quotes back when an app tries to
// publish the boot partition. It travels in the file rather than being
// looked up by whoever reads it, so a reader compiled long before this
// wording existed still explains the refusal in these words — see
// internal/devreserve's package doc for why that matters when the two ends
// are different releases.
const bootPartitionRole = "the boot partition this device started from"

// reserveDevices publishes the block devices GoSD keeps for the board's own
// operation, so that gadget.MassStorage — and anything else that hands a
// whole volume to another computer — refuses them by rule rather than by
// the accident of one happening to be mounted at the time (bean gosd-ix0r).
//
// Only the boot partition is reserved today. It carries the kernel this
// device starts from and the config tree that provisions it, so a USB host
// given write access to it has code execution on the next boot. Naming that
// one partition is also what refuses the whole card: a LUN over the disk
// contains the partition, which is the direction devreserve.Covers checks.
//
// The DATA partition is deliberately absent from the list. It is the app's
// own persistent storage, and an app may legitimately hand it to a computer
// to be edited — examples/usbwebsite does exactly that behind an operator
// opt-in, the vehicle bean gosd-4ajn locked in so the eMMC-less Pi Zeros
// can exercise the mass-storage path at all. What is on it that should not
// be published is gosd-init's own config store, and the answer to that is
// bean gosd-onjv moving the store to a partition of its own: reserving it
// is one more entry in this slice, refused by every already-compiled app
// without the gadget package changing at all.
func reserveDevices(deps Deps, log func(format string, args ...any), bootDevice string) {
	if deps.ReserveDevices == nil {
		return
	}

	devices := []devreserve.Entry{{Path: bootDevice, Role: bootPartitionRole}}
	if err := deps.ReserveDevices(devices); err != nil {
		log("publishing the devices an app must never share (%s) failed; gadget.MassStorage falls back to its mounted-device check alone: %v", devreserve.Path, err)
		return
	}
	log("reserved %s: an app may not offer it to a USB host", bootDevice)
}
