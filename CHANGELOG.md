## 0.6.5 (2026-08-18)

### Features

#### The status LED's running and fatal states have changed

v0.6.4 introduced the status LED with a fast blink for a recorded fatal
error. On real hardware that signal does not exist: `gosd-init` halts the
board immediately after recording the error, and a halted kernel stops
driving the LED, so the fast blink lasted about a tenth of a second and then
went dark. The release notes claimed it kept blinking through the halt. It
did not.

The three states are now:

| State | LED | Previously |
| --- | --- | --- |
| Booting | Flashes evenly, 250ms on/off | unchanged |
| Running | Blips briefly, 50ms on / 950ms off | solid on |
| Fatal | Solid on | fast 125ms blink |

Fatal is steady because a steady level is the only thing that can outlive the
halt. Running became a blip so that it stays clearly distinct from a solid
LED, and so a healthy board reads as alive rather than merely lit.

If you were relying on the previous meanings — solid for healthy, fast blink
for broken — those two have effectively swapped, and a device that has
halted is now the one showing a steady light.

One caveat worth knowing: for the fatal state to survive the halt at all, a
board's device tree has to mark the LED as retaining its state through
shutdown. No board ships that yet, so for now the LED still goes dark once
the device halts. Everything before the halt is unaffected, and the
[status LED guide](docs/status-led.md) tracks where this stands.

## 0.6.4 (2026-08-17)

### Features

#### Boot state shows on your board's onboard status LED

A GoSD device is headless, and until now there was no way to tell "still
booting" from "wedged" from "running fine" without a serial cable. Every
supported board has at least one software-controllable LED, and `gosd-init`
now uses it as that signal automatically — no code changes, no config:

- Blinks slowly (250ms on/off) while booting.
- Blinks fast (125ms on/off) if a [fatal error was recorded](docs/crash-reports.md).
- Solid on once your app has started and been handed control.

`gosd-init` picks the LED marked ACT, or the activity/status LED, or the
green LED, or whichever LED the board has, following its own device tree —
the [full selection rule and per-board table](docs/status-led.md) covers
which LED that is on each board. The blink itself is driven by the kernel's
own `timer` trigger, not a goroutine, so it keeps blinking through a fatal
halt or a wedged `gosd-init`.

## 0.6.3 (2026-08-17)

### Features

#### `gosd version` says which board artifacts your images will be built from

`gosd` had no way to report its own version, and no way at all to answer the
question that decides whether an image boots: which release of board kernels
and bootloaders it downloads.

```console
$ gosd version
gosd:      v0.6.2
artifacts: v0.10.2
go:        go1.26.5
```

`gosd --version` prints the same. A binary built from a checkout reports its
commit and whether the tree was modified, so "it works on my machine" is
answerable. When a board boots with one `gosd` and not another, the artifacts
line is usually where they differ.

### Fixes

#### Board images are now built from artifacts v0.10.2

`gosd build` downloads the board kernels and bootloaders published as
v0.10.2, up from v0.10.0, which brings:

- Cubie A5E images now boot the 1GB RAM variant
- The Cubie A5E kernel build now produces a USB-gadget variant device tree
- Cubie A5E U-Boot no longer scans USB on every boot

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
