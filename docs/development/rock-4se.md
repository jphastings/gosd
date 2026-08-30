# Developing for the Radxa ROCK 4SE (`rock-4se`)

Bench/bring-up knowledge from the initial board-support work (epic
`gosd-cuym`) that isn't captured elsewhere. Locked design decisions live in
CLAUDE.md; this file is for things a future agent or developer would
otherwise have to rediscover by hand. This was the first GoSD board ever
booted on real hardware (bean `gosd-sz6p`), so a couple of the lessons below
are general bring-up technique, not ROCK-4SE-specific — later boards' own
dev docs already point back here for them.

## Capturing serial on macOS: wire TX-only, and don't trust `tio` at 1.5M baud

The board **will not cold-boot** if the serial adapter's TXD line is
connected: back-powering through the SoC's RX pin keeps the PMIC from a
clean power-on reset, so the board hangs before BootROM with no LEDs
advancing — unplugging the adapter mid-hang releases it. Wire **TX-only**:
GND → pin 6, board TX (pin 8) → adapter RXD, and leave board pin 10 /
adapter TXD disconnected. GoSD has no interactive serial console, so nothing
is lost by never wiring host-to-board.

Separately, `tio` 3.9 on macOS silently fails to set 1,500,000 baud — Apple's
CP210x driver rejects termios rates above 921,600, but `tio` still prints
"Connected" and then reads nothing, forever, with no error. Capture via the
`IOSSIOSPEED` ioctl instead, after a normal raw termios setup:

```python
fcntl.ioctl(fd, 0x80045402, array('i', [1500000]))
```

The CP2102N adapter hardware itself is fine at 1.5M (loopback-verified both
rates) — this is a host-driver limitation, not a hardware one. Sporadic
RX-LED blinks on the adapter with zero bytes actually captured is the same
host-side baud problem, not a wiring fault.

## Boot-time baseline: 9.21s ± 0.02s, and where it goes

First successful boot (2026-07-23, bean `gosd-sz6p`) broke down as follows
(power-on to the first TPL serial byte, ~0.3s, is invisible to serial and
excluded):

| Phase | Duration |
|---|---|
| TPL (DDR init) | 0.26 s |
| SPL (FIT load + hash checks) | 0.41 s |
| ATF BL31 → U-Boot proper | 0.89 s |
| U-Boot device init → autoboot | 1.05 s |
| Bootflow scan (incl. the efi_mgr detour) | 0.46 s |
| Load `/Image` from SD | 3.02 s |
| Load initramfs + DTB → "Starting kernel" | 0.39 s |
| Kernel → first gosd-init output | 2.72 s |
| gosd-init → `/app` exec | 0.02 s |
| **Total, first TPL byte → app running** | **≈9.2 s** |

Repeated across 6 clean power cycles: 9.21s ± 0.02s, no panics, no oops. The
efi_mgr detour's 0.46s and its boot-log noise were removed from shipped
U-Boot builds after this baseline was captured (bean `gosd-k2i7` — see the
next section), so a fresh measurement would read a little faster and a lot
quieter than the log that produced this table. The biggest targets left for
anyone picking up boot-time optimization: the kernel `Image` load from SD
(3.0s — possibly a U-Boot MMC bus-mode question) and the kernel→gosd-init
gap (2.7s).

## Benign boot noise vs. an actual problem

The first hardware boot's serial log carries several alarming-looking lines.
Reading them correctly:

- `rockchip-pcie: PCIe link training gen1 timeout!` with no drive fitted —
  confirmed benign: it disappears entirely once an NVMe SSD is in the M.2
  slot. It's empty-slot probing, not a real link failure.
- `Card did not respond to voltage select! : -110` (twice) — this board has
  no onboard eMMC, so this is U-Boot probing an always-empty eMMC slot.
  Left alone deliberately (bean `gosd-k2i7`): some units may have an eMMC
  module fitted, and this is legitimate probing for that case.
- `udc fe800000.usb: failed to start g_mass_storage: -22` — the legacy
  in-kernel mass-storage gadget auto-probing and failing at boot. This
  turned out to be a fleet-wide defconfig leak (all six boards at the time,
  not Rockchip-specific as first suspected) and was fixed by disabling
  `CONFIG_USB_MASS_STORAGE` everywhere (bean `gosd-z9l4`); the
  configfs-based gadget stack GoSD actually uses was never affected by it.
- The EFI boot-manager detour (`Cannot persist EFI variables...`,
  `Loading Boot0000 'mmc 1' failed`, `Boot failed (err=-14)`) before
  extlinux is tried — cosmetic, and disabled in shipped U-Boot builds since
  (bean `gosd-k2i7`, which also happens to remove the voltage-select probe
  above as a side effect of the same fix, without deliberately targeting it).

Also worth knowing when reading a bring-up log: a deferred mDNS responder
("will retry on the next network change") can recover silently once the
link comes up — there's no follow-up success line on serial — so verify
with `curl`/`dig` against the real network rather than reading serial
silence as failure.

## The USB OTG port's physical identity, resolved

The kernel-build DTS patch that sets `dr_mode = "peripheral"` on
`usbdrd_dwc3_0` was written as a best guess: mainline's shared
`rock-pi-4.dtsi` treats both dwc3 controllers symmetrically, so DTS text
alone couldn't say which one is wired to the board's physical
hardware-switch OTG port. Hardware-verified (bean `gosd-sz6p`):
`usbdrd_dwc3_0` (`0xfe800000`) **is** the top/upper blue USB 3.0 port — the
one furthest from the board edge. The OTG mode switch itself is unmarked on
the PCB; position **away from the Ethernet jack selects device/peripheral
mode** (Radxa's own docs call this the "H" side on the related ROCK Pi 4
A/B). Confirmed via a CDC-ACM gadget enumerating and echoing back over
serial from macOS.

## NVMe: the feared PCIe quirk didn't show up, and the link trains at gen1

Pre-bring-up planning flagged "RK3399 PCIe link-training quirks with some
drives" as a real enough risk to test against the actual betamin SSD rather
than an arbitrary drive. It didn't materialize: a KIOXIA XG7000-512
(512GB, a PCIe4 drive) enumerated as `/dev/nvme0n1` with **0s** delay and no
link-retries, and sustained a 256MiB sequential read at **840MB/s** —
saturating what this board/controller combination actually trains at, PCIe
**gen1** ×4, well under what the drive itself is capable of. exFAT mounts
via `unix.Mount` and persists across unmount/remount.

## The ES8316 boots muted in two independent ways at once

Confirmed by the audibility pass's own control dump during first playback
(bean `gosd-cfkd`) — raising the volume alone would **not** have produced
sound, because both the level and the DAPM route were shut:

```
numid=2  "Headphone Mixer Volume":                 0,0 -> 8,8     (level, raised to >=75% of 0..11)
numid=4  "DAC Playback Volume":                    0,0 -> 144,144 (level, raised to >=75% of 0..192)
numid=33 "Left Headphone Mixer Left DAC Switch":   0 -> 1         (unmutes the playback path)
numid=35 "Right Headphone Mixer Right DAC Switch": 0 -> 1         (unmutes the playback path)
```

Worth knowing if you're debugging a similarly-silent codec on a different
board: two volume controls at zero *and* two DAPM mixer switches off, at the
same time, on the same signal path. The same dump also shows the pass's
matching is conservative: `Differential Mux` (the line-input selector) was
left untouched, and `Headphone Playback Volume` — already at its maximum —
was not rewritten. It only raises what's actually low, and never touches the
capture side.

## HDMI device selection needed the card identity, not the PCM name

`sound.Options{Prefer: HDMI}` silently opened a `snd-aloop` loopback instead
of this board's real HDMI sink (bean `gosd-qfgl`) — a bug in its own right,
but the part specific to this board is why `Prefer` couldn't rescue it: the
HDMI PCM's ALSA name is `ff8a0000.i2s-i2s-hifi i2s-hifi-0`, inherited from
its DAI-link name, with no "hdmi" substring anywhere in it. Detection logic
keyed on the PCM's own name will miss this board's HDMI device; it has to
look at the *card's* id/driver/name (`hdmisound`/`hdmi-sound`) from
`/proc/asound/cards` instead.

## Blob-free U-Boot: a toolchain-specific BL31 link failure, and a tag-pinning gotcha

This is GoSD's first blob-free Rockchip board — TF-A's BL31 is compiled from
source rather than pulled from rkbin — and building it hit two things worth
knowing if you're using `build/boards/rock-4se/uboot/` as the template for a
future RK3399-class board, which is what it's for (bean `gosd-dtpo`):

- **TF-A v2.15.0 fails to link BL31 for rk3399 with a stock Debian bookworm
  toolchain**: `region 'PMUSRAM' overflowed by 3928 bytes`. Root cause: TF-A
  commit `6c2e5bf68955` ("use clang as a linker", shipped in v2.13+) routes
  linking through the compiler driver, which mis-places `.pmusram` with
  Debian/Linaro GNU toolchains specifically (upstream:
  ARM-software/tf-issues#650, Debian bug #1118651). The fix —
  `LD=aarch64-linux-gnu-ld` on the TF-A `make`, restoring the direct-ld path
  that commit still supports — is one line, already applied and commented in
  the Dockerfile; called out here because it will recur verbatim on the next
  board built from this template.
- **`git ls-remote --tags` on an annotated tag returns the tag object's
  hash, not the commit it points to.** TF-A v2.15.0's tag object is
  `9ad327a8d124…`; the peeled commit a clone actually checks out is
  `da738d5eae93…`. `manifest.json` pins the peeled commit, which is what the
  Dockerfile verifies HEAD against — if you're re-deriving a pin from
  `ls-remote` output for a new board, dereference the tag (`^{}`) before
  treating its hash as a commit.

## Sound and HDMI-DRM remain unfinished

The analog jack path is hardware-verified (above); the HDMI audio variant
has never been heard on this board, and its radxa-zero-3e counterpart has
never even been built (bean `gosd-lrxz`). SPDIF is present in the device
tree but left disabled (`&spdif` never set `"okay"`) — the header's
SPDIF_TX pins would need their own DTS patch if anyone wants that path.
Microphone capture (the 4-ring jack and I2S mics off the headers) was
scoped and then deliberately scrapped as unwanted work with no consumer
(bean `gosd-tjrw`): the ALSA capture path is the same ioctl mechanism
`sound/` already speaks, just with `READI_FRAMES` against the codec's
capture node — small and well-understood whenever something actually needs
a microphone.
