---
# gosd-sz6p
title: 'ROCK 4SE: hardware bring-up and boot-time measurement'
status: completed
type: task
priority: normal
created_at: 2026-07-13T13:18:13Z
updated_at: 2026-07-23T14:28:42Z
parent: gosd-cuym
blocked_by:
    - gosd-h8a8
---

**First real-hardware bring-up of any GoSD board** — JP has the ROCK 4SE. Every COMPATIBILITY ✅ is code-complete-only today; expect this bean to shake out fleet-wide issues. Deviations become new beans, not inline fixes.

## Checklist

- [x] Flash SD, capture full serial boot log (UART2 @ 1500000 baud) into this bean
- [x] GbE: DHCP lease, mDNS resolution, HTTP reachable
- [x] 5× power-cycle survival
- [x] NVMe: /dev/nvme0n1 enumerates; exFAT mount via throwaway app (unix.Mount); read-throughput sanity — **use the actual betamin SSD** (RK3399 PCIe link-training quirks with some drives are a known risk)
- [x] Header I2C visible via examples/i2cscan; GPIO via gpioinfo
- [x] USB gadget mode reachable on the OTG port (existing serial/Ethernet functions)
- [x] Boot-time baseline: power-on → /app exec via serial timestamps, recorded here as the baseline for a later dedicated boot-optimization bean

## Bring-up session 2026-07-23 — first successful boot

**rock-4se boots end-to-end on real hardware**: U-Boot TPL/SPL → ATF → U-Boot →
extlinux → kernel → gosd-init → /app (examples/hello), from a stock `gosd build
--board rock-4se` on main using the published v0.5.0 artifacts. First GoSD board
ever to boot outside qemu. No Ethernet was available this session — network
items remain unchecked.

### Serial-capture lessons (macOS host) — read before the next board bring-up
- **The board fails to cold-boot with the adapter's TXD connected** (back-powering
  through the SoC RX pin keeps the PMIC from a clean power-on reset; board hangs
  before BootROM, no LEDs advance; unplugging the adapter mid-hang releases it).
  Wire **TX-only**: GND→pin 6, board TX pin 8→adapter RXD, board pin 10/adapter
  TXD left empty. gosd has no interactive serial, so nothing is lost.
- **tio 3.9 on macOS silently fails to set 1500000 baud** (Apple's CP210x driver
  rejects termios rates above 921600; tio still prints "Connected" and then
  reads nothing forever). Capture via the IOSSIOSPEED ioctl instead:
  `fcntl.ioctl(fd, 0x80045402, array('i', [1500000]))` after raw termios setup.
- CP2102N adapter hardware is fine at 1.5 M (loopback-verified both rates).
- Sporadic RX-LED blinks on the adapter with zero bytes delivered = host-side
  baud problem, not wiring.

### Boot-time baseline (boot 1; t=0 at first U-Boot TPL byte)
BootROM time before serial starts is invisible (~0.3 s, not included).

| Phase | Duration |
|---|---|
| TPL (DDR init) | 0.26 s |
| SPL (FIT load + hash checks) | 0.41 s |
| ATF BL31 → U-Boot proper | 0.89 s |
| U-Boot device init → autoboot | 1.05 s |
| Bootflow scan (incl. failed efi_mgr detour) | 0.46 s |
| Load /Image from SD | 3.02 s |
| Load initramfs + DTB → "Starting kernel" | 0.39 s |
| Kernel → first gosd-init output | 2.72 s |
| gosd-init → /app exec | 0.02 s |
| **Total, first TPL byte → app running** | **≈9.2 s** |

Biggest later optimization targets: kernel /Image load from SD (3.0 s — U-Boot
MMC bus mode?), kernel→init (2.7 s), U-Boot init (1.05 s), ATF handoff (0.89 s),
efi_mgr detour (0.46 s, see gosd-k2i7).

### Deviations → beans
- Legacy g_mass_storage gadget built into 4 boards' kernels, probes and fails at
  boot, can contend for the UDC → **gosd-z9l4**
- U-Boot efi_mgr detour + EFI/env/eMMC boot noise → **gosd-k2i7**
- `rockchip-pcie: PCIe link training gen1 timeout` with empty NVMe slot —
  presumed expected; re-evaluate during the NVMe checklist item with the betamin
  SSD attached.
- gosd-init probes /dev/mmcblk0p1 first, finds boot partition on the next
  candidate — fallback working as designed, no bean.
- No-network degradation is clean: WiFi skipped (no nl80211 — expected, no
  driver), mDNS defers with "will retry on the next network change", app starts
  regardless.

### Full serial boot log (boot 1, cleaned)
```

U-Boot TPL 2026.04 (Jul 16 2026 - 23:13:32)
lpddr4_set_rate: change freq to 400MHz 0, 1
Channel 0: LPDDR4, 400MHz
BW=32 Col=10 Bk=8 CS0 Row=16 CS=1 Die BW=16 Size=2048MB
Channel 1: LPDDR4, 400MHz
BW=32 Col=10 Bk=8 CS0 Row=16 CS=1 Die BW=16 Size=2048MB
256B stride
lpddr4_set_rate: change freq to 800MHz 1, 0
Trying to boot from BOOTROM
Returning to boot ROM...

U-Boot SPL 2026.04 (Jul 16 2026 - 23:13:32 +0000)
Trying to boot from MMC2
## Checking hash(es) for config config-1 ... OK
## Checking hash(es) for Image atf-1 ... sha256+ OK
## Checking hash(es) for Image u-boot ... sha256+ OK
## Checking hash(es) for Image fdt-1 ... sha256+ OK
## Checking hash(es) for Image atf-2 ... sha256+ OK
## Checking hash(es) for Image atf-3 ... sha256+ OK
## Checking hash(es) for Image atf-4 ... sha256+ OK
load_simple_fit: Skip load 'atf-5': image size is 0!
NOTICE:  BL31: v2.15.0(release):v2.15.0
NOTICE:  BL31: Built : 23:13:12, Jul 16 2026


U-Boot 2026.04 (Jul 16 2026 - 23:13:32 +0000)

SoC: Rockchip rk3399
Reset cause: POR
Model: Radxa ROCK 4SE
DRAM:  4 GiB (total 3.9 GiB)
PMIC:  RK808 
Core:  310 devices, 34 uclasses, devicetree: separate
MMC:   mmc@fe310000: 2, mmc@fe320000: 1, mmc@fe330000: 0
Loading Environment from MMC... Reading from MMC(1)... *** Warning - bad CRC, using default environment

In:    serial,usbkbd
Out:   serial,vidconsole
Err:   serial,vidconsole
Model: Radxa ROCK 4SE
Net:   eth0: ethernet@fe300000
[?25h
[2KHit any key to stop autoboot: 0
Scanning for bootflows in all bootdevs
Seq  Method       State   Uclass    Part  Name                      Filename
---  -----------  ------  --------  ----  ------------------------  ----------------
Scanning global bootmeth 'efi_mgr':
7[r[999;999H[6n8Card did not respond to voltage select! : -110
Card did not respond to voltage select! : -110
Cannot persist EFI variables without system partition
  0  efi_mgr      ready   (none)       0  <NULL>                    
** Booting bootflow '<NULL>' with efi_mgr
Loading Boot0000 'mmc 1' failed
EFI boot manager: Cannot load any image
Boot failed (err=-14)
Scanning bootdev 'mmc@fe320000.bootdev':
  1  extlinux     ready   mmc          1  mmc@fe320000.bootdev.part /extlinux/extlinux.conf
** Booting bootflow 'mmc@fe320000.bootdev.part_1' with extlinux
1:	gosd
Retrieving file: /Image
Retrieving file: /initramfs.cpio.zst
append: console=ttyS2,1500000n8 quiet init=/init gosd.board=rock-4se
Retrieving file: /rk3399-rock-4se.dtb
## Flattened Device Tree blob at 12000000
   Booting using the fdt blob at 0x12000000
Working FDT set to 12000000
   Loading Ramdisk to f0446000, end f0bff59a ... OK
   Loading Device Tree to 00000000f0ea5000, end 00000000f0eb7625 ... OK
Working FDT set to f0ea5000

Starting kernel ...

[    1.564399] no file given for LUN0
[    1.564723] udc fe800000.usb: failed to start g_mass_storage: -22
[    1.565269] g_mass_storage gadget.0: probe with driver g_mass_storage failed with error -22
[    2.350044] rockchip-pcie f8000000.pcie: PCIe link training gen1 timeout!
[    2.350712] rockchip-pcie f8000000.pcie: probe with driver rockchip-pcie failed with error -110
[    2.477440] /dev/mmcblk0p1: Can't lookup blockdev
[gosd] hostname set to "hello"
[gosd] boot partition mounted at /boot
[gosd] hostname from gosd.toml
[gosd] hostname set to "hello" (gosd.toml applied)
[    2.482725] /dev/mmcblk0p2: Can't lookup blockdev
[    2.483596] /dev/mmcblk1p2: Can't lookup blockdev
[    2.484045] /dev/vda2: Can't lookup blockdev
[gosd] no data partition on this image; mounting /data read-only
[gosd] started /app (pid 155)
[gosd] WiFi unavailable, skipping: opening nl80211: netlink receive: no such file or directory
[gosd] mdns: no responder yet for hello.local (starting mDNS responder for hello.local: no usable interfaces found for mDNS); will retry on the next network change
gosd hello, host=hello board=rock-4se boots=no-data-partition
```

### Power-cycle survival + baseline consistency (2026-07-23)

6 cold boots captured in one serial session (JP reported 5 power-ons; 6 full
boot sequences appear in the capture — one cycle likely bounced). Every boot
reached the app. Timing is extraordinarily consistent:

| Boot | TPL→kernel | TPL→app |
|---|---|---|
| 1 | 6.48 s | 9.21 s |
| 2 | 6.47 s | 9.20 s |
| 3 | 6.48 s | 9.21 s |
| 4 | 6.48 s | 9.23 s |
| 5 | 6.47 s | 9.20 s |
| 6 | 6.47 s | 9.22 s |

**Baseline: 9.21 s ± 0.02 s, first TPL serial byte → app exec.** No panics, no
oops, no new errors across boots — the only recurring error lines are the three
already tracked (efi_mgr detour → gosd-k2i7, g_mass_storage → gosd-z9l4,
rockchip-pcie link timeout with empty NVMe slot — revisit with SSD attached).

### GbE + first NVMe-populated boot (2026-07-23, boot 7)

Ethernet + betamin NVMe SSD attached, same hello image:
- **PCIe link-training timeout GONE** with the SSD in the slot — the empty-slot
  timeout was indeed benign probing noise.
- **GbE all green**: DHCP lease 192.168.1.224; `hello.local` resolves via mDNS
  from macOS; `curl http://hello.local/` returns the app response — which
  arrived over IPv6, so both stacks work.
- App started before link-up (by design); gosd-init logged mDNS deferral
  ('will retry on the next network change') and then recovered *silently* —
  no serial line for the eventual DHCP lease or mDNS responder start. Works,
  but a success log line would improve bring-up/debug ergonomics; consider a
  small follow-up bean.

### NVMe with the betamin SSD (2026-07-23, boot 8) — 7/7 pass

Throwaway nvmetest app (embedded 64 MiB exFAT image; source preserved below in
spirit — scratchpad is ephemeral): enumerate, identify, sequential read,
raw-write exFAT image, unix.Mount exfat, file round-trip, remount persistence.

- /dev/nvme0n1 appeared **0 s** after app start; no link-training retries with
  this drive — the feared RK3399 PCIe quirk did not manifest.
- Drive: KIOXIA XG7000-512 2242, 512 GB, fw SN13683.
- Sequential read: **256 MiB @ 840 MB/s** (≈ saturating PCIe gen1 ×4).
- exFAT (CONFIG_EXFAT_FS=y) mounts via unix.Mount; files persist across
  unmount/remount. Drive now contains the 64 MiB test filesystem (GOSDTEST).
- Bonus findings: app crash-loop (my throwaway's `select {}` deadlock after
  completion) demonstrated **gosd-init's restart policy working on real
  hardware** — app restarted ~1 s after exit, indefinitely, cleanly.
- Deviation → bean **gosd-6h1x**: image built with `gosd build .` advertised
  `app.local` — default hostname falls back to 'app' for pkgPath '.'.
- mDNS re-verified with second hostname: app.local → 192.168.1.224.

### I2C via examples/i2cscan (2026-07-23, boots 9-10) — bus mappings hardware-confirmed

SparkFun Qwiic Button (addr 0x6f) used as the probe device; i2cscan image,
gosd-init's restart loop conveniently re-scans every few seconds (backoff
1s→2s→4s… observed working; run-once app exits 0, restarted cleanly).

- Header pins 27/28 → button ACKs on **/dev/i2c-2** (i2c2, SDA2/SCL2) —
  matches the DTS patch comment exactly.
- Header pins 3/5 → button ACKs on **/dev/i2c-7** (i2c7, Pi-position, the
  bus the PN532 will use) — matches the patch comment exactly.
- Adapter numbering is alias-pinned to controller names (buses present:
  0,1,2,3,4,6,7 — gaps at 5/8 = disabled controllers, numbering stable).
- Internal buses observed: i2c-0 EEPROM aliasing 0x50-0x57, i2c-1 ES8316
  codec at 0x11.
- Header pins 29/31 → button ACKs on **/dev/i2c-6** (i2c6, SCL6/SDA6) —
  matches the patch comment. All three header buses hardware-verified.

Feeds gosd-x59n (docs rows can say 'hardware-verified' for i2c2/i2c7).

### USB gadget on the OTG port (2026-07-23, boots 12-13) — pass, open question resolved

- examples/usbwebsite degrades gracefully on this board: 'no onboard eMMC on
  this board; this example needs...' + clean exit — correct behavior, ROCK 4SE
  has no eMMC module. Used examples/usbserial (built with --usb-gadget) for
  the actual gadget test.
- CDC-ACM gadget enumerated on macOS as /dev/cu.usbmodem111401; line sent from
  the Mac echoed back intact. Serial log: 'gadget applied, waiting for
  /dev/ttyGS0'.
- **Resolves patch 0003-usb-dwc3-peripheral.patch's 'WHICH PHYSICAL PORT
  (UNRESOLVED)' comment**: usbdrd_dwc3_0 (0xfe800000) is the TOP blue USB 3.0
  port (furthest from PCB). The 4SE's OTG switch is unmarked on the PCB;
  hardware-verified: position AWAY from the Ethernet jack = device/peripheral
  mode (Radxa wiki calls this the 'H' side on ROCK Pi 4 A/B). Patch comment
  update folded into gosd-z9l4's artifact-rebuild PR.

## Summary of Changes

First-ever real-hardware bring-up of a GoSD board: all 7 checklist items pass
on the Radxa ROCK 4SE using stock `gosd build` images from main (v0.5.0
artifacts, real download path). Full serial boot logs, a 9.21 s ± 0.02 s
boot-time baseline (6 boots), GbE/mDNS/HTTP (IPv4+IPv6), NVMe at 840 MB/s
with exFAT via unix.Mount, all three header I2C buses device-verified
(Qwiic Button at 0x6f), all five GPIO banks enumerated, and a CDC-ACM USB
gadget echo round-trip — details in the session sections above.

Deviations filed as beans, not fixed inline:
- gosd-z9l4 — legacy g_mass_storage built into 4 boards' kernels (+ fold-in:
  0003 patch comment update now the OTG port question is resolved)
- gosd-k2i7 — U-Boot efi_mgr detour + boot noise
- gosd-6h1x — `gosd build .` hostname falls back to 'app'
- gosd-x59n — docs/runtime.md has no rock-4se peripheral rows (now with
  hardware-verified pin mappings to use)

Host-side capture lessons (tio 1.5M-baud silent failure → IOSSIOSPEED;
back-powering → TX-only serial wiring) recorded above and in agent memory.
Hardware state at close: SD carries the usbserial test image; the betamin
NVMe carries the 64 MiB GOSDTEST exFAT test filesystem.
