# Developing for the Raspberry Pi Zero W (`pi-zero-w`)

Bench/bring-up knowledge from the initial board-support work (epic
`gosd-ajpz`) that isn't captured elsewhere. Locked design decisions live in
CLAUDE.md (the DMA `dma-ranges` patch, the mainline-vs-downstream DTB split,
the SPI `__overrides__` gap, `SERIAL_8250_RUNTIME_UARTS`) and in
[this board's kernel README](../../build/boards/pi-zero-w/README.md)
(kernel build mechanics, toolchain, defconfig); this file is for things a
future agent or developer would otherwise have to rediscover by hand.

## WiFi enumeration needs `CONFIG_MMC_SDHCI_IPROC` — and phantom radios get there first

Before this board's WiFi worked at all, `nl80211 CONNECT` was rejected with
`EINVAL` on every join attempt, which looked exactly like a firmware
capability wall (the 43430 chip's firmware genuinely lacking the offloaded
4-way handshake). It wasn't: `bcmrpi_defconfig`'s `mac80211_hwsim` gets
promoted from `=m` to `=y` by the no-modules build and creates simulated
`wlan0`/`wlan1` interfaces with no handshake offload at all, and `wifiup`
picked one of those before the real radio ever showed up. The real radio
hadn't enumerated because this board's mainline-style device tree puts the
WiFi SDIO controller on a `brcm,bcm2835-sdhci` node, whose only driver at the
pinned commit is `sdhci-iproc` (`CONFIG_MMC_SDHCI_IPROC`) — a symbol
`bcmrpi_defconfig` doesn't set, since it assumes the downstream `bcm2835-mmc`
driver, which can't bind this DT at all. Net effect: zero `brcmfmac` dmesg
lines, and the phantom hwsim interface silently absorbing every connect
attempt.

Fixed by adding `CONFIG_MMC_SDHCI_IPROC=y` and disabling `mac80211_hwsim` in
`build/boards/pi-zero-w/kernel.fragment`. `wifiup` also gained a permanent
hardening from this: a `SupportsOffloadedHandshake` check
(`NL80211_EXT_FEATURE_4WAY_HANDSHAKE_STA_PSK`) before every PSK `CONNECT`,
so a phantom or capability-limited radio produces an actionable log line
naming the interface and the missing feature instead of a bare `EINVAL`
retry loop. Bean `gosd-6nl2` has the full nl80211/brcmfmac source citations
if this class of bug resurfaces on a future Broadcom board.

## go-diskfs's device-sizing ioctl silently truncated on this board's 32-bit arch

`go-diskfs@v1.9.3` sized a block device with
`unix.IoctlGetInt(fd, unix.BLKGETSIZE64)`, reading into a native Go `int`.
On `GOARCH=arm` (this is the only 32-bit board in the fleet) that's a 4-byte
destination, but the kernel's `BLKGETSIZE64` handler always writes 8 bytes —
corrupting adjacent stack memory and truncating the reported size of any
device ≥ 4 GiB to a fraction of its real capacity. `emmc` never hit this on
pi-zero-w (no onboard eMMC to format), but the `disk` package does the
moment a USB drive is attached. Fixed by removing the dependency on
go-diskfs's own device-open/sizing path entirely — `internal/diskfmt`'s
`openDisk` sizes via `lseek(SEEK_END)`, which is 64-bit correct on every
architecture Go supports — rather than patching the ioctl call itself. Worth
remembering if a future dependency (or a new go-diskfs release) reintroduces
a similarly word-sized syscall: this is the one board where that class of
bug is even observable. Bean `gosd-fjio`.

## `gosd build-external`/`--with-external` cannot catch a wrong `GOARM`

`internal/staticelf`'s verification (ELF `Class`/`Machine`, no `PT_INTERP`)
accepts a `GOARM=5`, `6`, or `7` binary interchangeably: Go's linker emits
identical `e_flags` (`0x5000002`) regardless of `GOARM`, and — contrary to
what you'd expect — emits no `.ARM.attributes` section at all for any
`GOARM` value, so there's no `Tag_CPU_arch` to fall back to either. A
companion binary cross-compiled with Go's default `GOARM=7` for this board
passes verification cleanly and then faults with an illegal instruction on
real armv6 silicon. This is recorded as a permanent, documented gap
(`internal/staticelf`'s package doc and
[the externals guide](../externals.md)), not something pending a fix —
there is no ELF-header-only way to detect it. Get `GOARM=6`
right at the cross-compile step for any `build-external`/`--with-external`
binary targeting this board; nothing downstream will catch a mistake. Bean
`gosd-aur4`.

## ext4 `/data` is bench-verified on real hardware, including cross-board adoption

Bean `gosd-58p6` (closed 2026-08-10) proved this board's
`--data-filesystem=ext4` path end-to-end on real hardware, with a working
serial console watching every step: the 512 MiB golden image writes,
`EXT4_IOC_RESIZE_FS` grows it to the card's full size (14.6 GiB in the
test), the volume mounts read-write, and a hard power cut mid-write
(`sdwire power off`, no clean shutdown) survives via journal replay on the
next boot (boot counter continued cleanly; the card reported "already
present" rather than triggering a reformat). The same session then went
further than any other board's ext4 verification: the pi-zero-w-formatted
volume was adopted intact by a pi-3b (arm64) image sharing its data label —
confirming the on-disk format really is architecture-independent, not just
asserted to be. If you touch COMPATIBILITY.md's `[^pi-data-ext4]` footnote,
note its current text ("Pi Zero W ... has never run ext4 on real hardware at
all") predates this bean and is stale — this bean is the evidence to fold
in.

## Boot-time baseline

Best-effort bench figures from `gosd-qltr`'s bring-up (kernel printk stamps,
serial capture, WiFi path, `examples/hello`): kernel to `gosd-init` ~6.9s,
app started ~7.2s, NTP-synced clock by ~boot+15s, power-on to
HTTP-reachable conservatively ≤45s. That's noticeably slower than the Zero
2W's ~25s, expected given this board's single armv6 core with no NEON. It
also misses the project's blanket <15s boot-time target, for the same
reason as the Zero 2W (`gosd-m9dj`): the target predates real hardware and
fits wired network paths, not WPA2's PBKDF2 cost over WiFi on a slow single
core. That's a known, accepted gap rather than a regression to chase — see
`gosd-qltr`'s acceptance note if re-scoping the target.

## A custom-kernel recipe that re-enables `SND` drags in the whole Pi audio zoo

Not a board-profile bug — a trap for anyone writing a `gosd build-kernel`
recipe for this board that needs `CONFIG_DRM_VC4` (a display recipe, for
example). `DRM_VC4` hard-depends on `SND && SND_SOC` in the pinned kernel
tree, so satisfying that dependency with three enable lines
(`CONFIG_SOUND=y`, `CONFIG_SND=y`, `CONFIG_SND_SOC=y`) is enough to trigger
the same defconfig-promotion trap CLAUDE.md documents elsewhere: `=m` audio
drivers in `bcmrpi_defconfig` — roughly forty Pi HAT machine drivers, their
ASoC codecs, MIDI gadget functions, the works — get promoted to `=y` even
though the recipe wants none of them. `examples/sattrack`'s
`pi-zero-w.fragment` needed an explicit ~70-symbol deny-list, individually
verified against the pinned tree's Kconfig, to actually get "just enough
for `DRM_VC4`'s dependency, nothing else." As of writing this fix is still
`in-progress` (bean `gosd-df57`) pending a `gosd build-kernel` re-run and
size re-measurement — check its status before assuming the deny-list has
been bench-verified.
