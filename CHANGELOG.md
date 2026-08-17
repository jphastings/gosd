## 0.6.2 (2026-08-17)

### Fixes

#### `--usb-gadget` now refuses for the Radxa Cubie A5E instead of building an image that can't work

Hardware testing showed the Cubie A5E cannot present itself as a USB device
at the currently pinned board artifacts: the USB-C port's host controllers
share a phy with the peripheral controller, and with nothing on the board to
arbitrate between them the host side wins every boot, so the port never
enumerates. The board's device tree pins peripheral mode, which is what
GoSD's earlier support claim was based on, but that is not enough on its own.

Building with `--usb-gadget` for this board now fails with an explanation
rather than producing an image that looks correct and cannot work. Support
returns once a board artifacts release carries the variant device tree that
disables those host controllers — at which point USB-C host mode becomes the
trade-off, since an image can serve one role or the other but not both. The
USB 3.0 Type-A port is unaffected.

#### Boards with no hardware entropy source now get a DHCP lease reliably

On a board whose kernel has no random-number source, the DHCP client could
fail to build its first packet at all — it drew the transaction ID from the
kernel's cryptographic pool, which stays unavailable for the first several
seconds of boot on such hardware. The board came up, started the app, and
silently never joined the network. Transaction IDs no longer depend on that
pool.

Separately, a board that cannot get an address now keeps reporting it on the
console at a backing-off interval, instead of logging one failure and going
quiet — so an unreachable board says why.

#### A data partition reformatted from ext4 to FAT32 no longer halts the device

Formatting a volume as FAT32 over a previous ext4 volume left the old ext4
superblock intact, because the FAT32 writer never touches the offset it sits
at. gosd-init then identified the dead filesystem in preference to the live
one and halted the board on its next boot, reporting corruption and the old
volume's label. Establishing a volume now clears any previous filesystem's
signatures first, so changing `--data-filesystem` between releases reformats
cleanly — as documented — rather than stopping a healthy device.

## 0.6.1 (2026-08-14)

### Features

#### Releases are now prepared by change files and a release PR

Each user-facing change ships a small markdown change file; a bot-maintained release PR accumulates them and, when merged, tags and publishes the CLI, artifacts, and npm releases with real release notes.
