---
gosd: minor
---

#### A USB mass-storage gadget can no longer be pointed at the boot partition

`gadget.MassStorage` now refuses to expose the partition your device booted
from, or the whole card holding it, to a USB host — whether or not it
happens to be mounted at the time. That partition carries the kernel the
board starts from and the config tree it was provisioned with, so a computer
given write access to it has code execution on the next boot.

Until now the only thing standing in the way was a check that the backing
path wasn't currently mounted, which worked purely because `gosd-init` keeps
`/boot` mounted for the life of the device. Nothing on a GoSD board could
identify the boot medium independently: the image boots from an initramfs,
so the kernel command line names no root block device. `gosd-init` now
publishes the devices it reserves as it boots, and the `gadget` package
refuses against that instead.

The refused error wraps a new `gadget.ErrReservedDevice`, so an app that can
do something else when a volume isn't available — offer a different one, or
run without the drive — can match it with `errors.Is` and degrade
gracefully, as `gadget.ErrNoController` already allows.

The **data partition is not refused**: it is your app's own storage, and
sharing it stays your call (`examples/usbwebsite` still offers it behind its
documented opt-in). Everything the runtime documentation says about what
lives on that partition, and why publishing it discloses this device's WiFi
passphrase and ingress tokens, still applies.

Apps built against this release keep working on images produced by an older
`gosd`, which publish no such list; there, `MassStorage` behaves exactly as
it did before.
