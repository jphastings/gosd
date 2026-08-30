# Developing for the Raspberry Pi 3B (`pi-3b`)

Bench/bring-up knowledge from the initial board-support work (epic
`gosd-xhc3`) that isn't captured elsewhere. Locked design decisions live in
CLAUDE.md and `build/boards/pi-3b/kernel.fragment`'s own header comments;
this file is for things a future agent or developer would otherwise have to
rediscover by hand. (The LED `GPIO_BCM_VIRT` fragment quirk, the
LAN9514/LAN7515-vs-`dwc_otg` binding story, and the maiden-boot 3B+/lan78xx
discovery are already covered in CLAUDE.md and COMPATIBILITY.md — not
repeated here.)

## Hardware bring-up is not finished

Bean `gosd-f5xm` is still in-progress: only the pre-release maiden-boot
session (a locally built kernel via `--artifacts-dir`, not the published
artifact) has run. Every checklist item that needs the real released image —
serial console `/proc/cmdline` capture, `gosd.toml` FAT edit cycle, I2C/SPI/
GPIO enumeration, boot-time baseline, and the WiFi item below — is still
open. Epic `gosd-xhc3` stays open for exactly this reason even though the
other three children (board profile, kernel build, artifacts activation) are
done. Treat anything below that depends on hardware verification as
provisional until that checklist closes.

One specific open item worth calling out: whether the Pi firmware injects
`8250.nr_uarts` on this board (as it does on the Zero 2W) or relies entirely
on the fragment's `CONFIG_SERIAL_8250_RUNTIME_UARTS=1` (as the Zero W had
to, per `gosd-md4w`) has never been checked against a real `/proc/cmdline`
capture — the maiden-boot session didn't get that far into the checklist.

## The bench unit on hand is a 3B+, which blocks the WiFi checklist item

The board used for bring-up so far is a 3B+ (rev `a020d3`, "RPI3BP"
silkscreen), confirmed by the firmware requesting
`bcm2710-rpi-3-b-plus.dtb` before falling back to the shipped
`bcm2710-rpi-3-b.dtb`. Its WiFi radio is a BCM43455, not the BCM43438 our
manifest's Cypress 43430 blob set targets — so the WiFi checklist item
cannot pass on this specific unit without adding the 43455 blobs and
`raspberrypi,3-model-b-plus` aliases (tracked as an epic-level follow-up in
`gosd-oq0z`, not yet scheduled). Completing that checklist item needs either
that follow-up or an actual 3B unit on the bench.

## Multi-DTB kernelspec mechanism: pi-3b is its first and only user

Shipping one image for both the 3B and the 3B+ needed a second DTB
(`bcm2710-rpi-3-b-plus.dtb`) alongside the board's primary `DTB` field.
Rather than generalizing `KernelSpec.DTB` into a slice, `gosd-oq0z` added a
narrow `AdditionalDTBs []DTB` field plus an `AllDTBs()` helper
(`internal/kernelspec/kernelspec.go`) that returns `DTB` followed by
`AdditionalDTBs`. `internal/kernelbuild` builds each distinct `make dtbs`
target once and copies out every listed DTB, and the cache key/cache-
completeness checks cover the extra blob (an old cache entry built before
this field existed is a cache miss, not a false hit). Every other board's
spec is untouched — if a future board needs more than one DTB, this is the
field to reach for, and `TestAdditionalDTBsOnlyOnExpectedBoards` in
`internal/kernelspec/kernelspec_test.go` is the drift guard that needs its
allowlist extended.

## A wrong assumption about WiFi firmware aliases, corrected

Bean `gosd-06kj`'s findings (from an earlier board's research) had a passing
remark that only `model-zero-2-w`, `3-model-b`, and `0-compute-module` carry
a `43430b0` blob alias in RPi-Distro/firmware-nonfree. Re-checking the full
`brcm/` directory listing at the pinned commit (`9794282e`) for this board's
manifest found that's wrong: **no `43430b0` alias exists for `3-model-b`** —
only `model-zero-2-w` has one. The directory listing is authoritative over
that earlier remark; the pi-3b manifest carries exactly three WiFi firmware
aliases (bin/clm_blob/txt), the same shape as pi-zero-w's, not four.

## Benign kernel-config noise after `olddefconfig`

Of the fragment's ~47 `# ... is not set` lines, only `CONFIG_DEBUG_KERNEL=y`
comes back re-enabled after `olddefconfig` runs during the real
`gosd build-kernel` build. This is expected, not a regression: pi-zero-2w's
and pi-zero-w's committed configs show the identical symbol re-appearing
(recorded in `gosd-s7fk`). If you spot-check a future pi-3b `kernel.config`
and see `CONFIG_DEBUG_KERNEL=y`, that alone isn't a sign the trim broke —
it survives on every Pi board in the fleet.
