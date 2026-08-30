# Developing for the Turing RK1 (`turing-rk1`)

Bench/bring-up knowledge from the initial board-support work (epic
`gosd-bntd`) that isn't captured elsewhere. Locked design decisions live
in CLAUDE.md; this file is for things a future agent or developer would
otherwise have to rediscover by hand. This board is hosted in a Turing Pi
2 — read [the shared Turing Pi 2 BMC notes](turing-pi-2-bmc.md) too, since
a fair amount of this bring-up's friction turned out to be baseboard/BMC
tooling, not the RK1 module itself.

## Two real DT-research-vs-hardware mismatches, both fixed

DT-only research (no hardware in hand yet) got two genuinely reasonable
guesses wrong. Both are fixed in code now, but the pattern is worth
internalizing for the *next* board: **a DT alias or a `status = "okay"`
node is a hypothesis about hardware behavior, not a fact, until a real
board confirms it.**

- **Console device name, not baud.** The DTS's `stdout-path` names the
  `serial9` alias, so the board profile shipped `console=ttyS9`. Real
  hardware panicked (`Warning: unable to open an initial console` →
  `Kernel panic - not syncing: Attempted to kill init!`). Root cause: the
  generic 8250/8250_dw serial driver does **not** number a UART by its DT
  alias index — this board's DTS has exactly one enabled UART node, so it
  registers as `ttyS0` regardless of the alias being named `serial9`. The
  baud rate (115200) was correct all along; only the device name was
  wrong. Fixed in `extlinux.conf.tmpl` (bean `gosd-vh82`).
- **USB gadget mode.** The DTS's OTG-capable port looked like a plausible
  gadget candidate from a DT read alone (`status = "okay"`, no explicit
  `dr_mode`, comment literally saying `/* USB 0: USB 2.0 only, OTG-capable
  */`). Hardware bring-up found it doesn't work: the port is bound to
  `&usb_host0_xhci`, and Linux's `xhci-hcd` driver is host-only *by design*
  — XHCI is a host-controller-interface spec with no gadget-mode
  implementation at all, independent of what `dr_mode` says or doesn't say.
  There's no dwc3/dwc2-style dual-role controller node anywhere in this
  DTS. The hardware PHY genuinely is OTG-capable; the *driver binding*
  mainline chose for it isn't. Fixed: `UsbGadgetSupport()` returns
  `Supported: false` (bean `gosd-tqme`). Whether a DTS patch could ever
  expose a gadget-capable binding on this SoC is an open, unresearched
  question — would need real RK3588 TRM/driver research, not guessing.

## Flashing this board needs a BMC firmware new enough to know about it

The BMC-driven flash path (`tpi flash`, and the raw REST API underneath
it) failed universally with "No supported devices found" on every node,
on BMC firmware v2.0.5. Root cause: v2.0.5 was the *first* BMC release to
add RK1 support at all (Nov 2023) and had known `rockusb`-driver bugs
(`READ_CAPACITY` handling). Upgrading to v2.3.4 (Feb 2025) fixed it
completely, on the first try, no other changes. If a fresh RK1 bring-up
hits this exact error, check the BMC firmware version before assuming
anything about the RK1 module, the image, or the flashing command is
wrong.

`tpi`'s own CLI proved unreliable to drive from a sandboxed
non-interactive shell (intermittent local-Keychain-access failures, not a
BMC problem — see [the shared Turing Pi 2 BMC notes](turing-pi-2-bmc.md)
for the `--user`/`--password` workaround that generalizes past this).
Before that was found, the whole flash/power/UART flow was driven
successfully via direct `curl` against the BMC's REST API instead:

```
opt=set&type=flash&file=X&length=N&sha256=H&node=N-1     # 0-indexed node!
POST .../upload/{handle}                                  # multipart
opt=get&type=flash                                         # poll for Done/Error
opt=set&type=power&node{1-indexed}=1                       # boot it
opt=get&type=uart&node=N-1                                 # read the console
```

Note the REST API's node parameter is **0-indexed** in some calls and
**1-indexed** in others (`node=N-1` for flash/uart vs. `node{N}=1` for
power) — this isn't a typo, it's the actual API shape found by
reverse-engineering, confirmed working end-to-end.

## Verified end-to-end on real hardware

With the console fix applied, a stock `gosd build --board turing-rk1`
image boots completely clean, no kernel-param workarounds: U-Boot SPL/
proper (FIT signature checks pass) → kernel 6.18.37 → gosd-init → the app.
Confirmed: boot partition mount, DHCP + mDNS (`hello.local` reachable over
HTTP), NTP time sync (no RTC, the designed fallback), data partition
persistence across a real power cycle in **both** FAT32 and ext4, and
NVMe-as-storage via the `disk` package (format/mount/grow/persistent
adoption) — once a known-good drive was used (see below). No status LED
found on this board, matching DT research (no `led`/`gpio-leds` node).

Boot-time baseline (best effort, power-on to app-reachable-over-HTTP,
`hello` example, no data partition): **~17s**, of which ~6s is
bootloader+kernel+gosd-init and the rest is the boot-time-measurement
script's own curl/DNS/polling overhead.

Benign warnings seen in dmesg, not investigated further, not blocking:
`sdhci-dwcmshc ...: Can't reduce the clock below 52MHz in HS200/HS400
mode`; `rockchip-pm-domain ...: Failed to create device link ... for
spi2.0`; assorted `/dev/mmcblkXp2`/`/dev/vda2` "Can't lookup blockdev"
lines during gosd-init's disk-candidate probing (expected — only one real
candidate exists per board, the rest are other boards' probe attempts
against devices that don't exist here).

## An NVMe drive that "doesn't work" may just be a bad drive

The first NVMe drive tried in this board's M.2 slot never trained a PCIe
link at all (`/sys/class/nvme` and `/dev/nvme*` both empty, no downstream
device answered). Before concluding this was a GoSD/kernel/DTS bug,
everything else was independently verified correct: regulator, reset/
clkreq/wake pinctrl, and Kconfig (`CONFIG_PCIE_ROCKCHIP_DW_HOST`,
`CONFIG_PHY_ROCKCHIP_SNPS_PCIE3`, `CONFIG_BLK_DEV_NVME`) all checked out.
A second, known-good drive in the exact same slot trained a full 8.0 GT/s
x4 link immediately and worked perfectly end-to-end (format, grow to the
disk's real ~469 GiB size, persistent adoption across a power cycle). The
first drive was genuinely faulty. **If NVMe shows zero PCIe link training
at all (not a driver error, just silence), suspect the drive before the
software stack** — a real driver/DTS bug on this class of hardware
tends to show partial enumeration or a specific error, not total silence.

A onboard PCIe/USB oddity worth knowing about if you go looking at
`/sys/bus/pci/devices` on this board: there's a second, always-present PCIe
device — a VIA Technologies USB3 xHCI controller (vendor `0x1106`, device
`0x3483`) on a separate 1-lane PCIe root complex, unrelated to the M.2
slot's own 4-lane controller. This matches Turing's own hardware
changelog text about a "Mini PCIe USB interface," and is plausibly the
same path the BMC uses internally for node flashing — it's not a second
NVMe candidate and its presence is expected, not a diagnostic signal.

## Kernel bloat: the defconfig-promotion trap hit here too

CLAUDE.md already documents "Pi defconfigs ship `=m` drivers that the
no-modules build promotes to `=y`" as a known trap for the Broadcom
family; the same class of thing happened here despite the fragment's
explicit DRM/video cuts — GPU/video-codec drivers (`rockchip-rga`,
`hantro-vpu`, `uvcvideo`) still showed up in dmesg on a board with no
display use case in scope. Tracked as a follow-up (bean `gosd-vo5q`), not
yet fixed as of this board's initial bring-up.
