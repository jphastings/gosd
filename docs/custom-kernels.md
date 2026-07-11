# Custom kernels: compiling in a driver GoSD's stock kernel cut

GoSD's published kernels are trimmed hard — no sound, no DRM/video/fb, no
camera, no filesystems beyond FAT/tmpfs — to keep boot fast and the image
small. Most apps never need anything the stock kernel lacks. When yours
does (a USB DVB-T tuner, a niche I2C/SPI sensor with its own kernel driver,
anything gated behind a Kconfig symbol GoSD cut), `gosd build-kernel` lets
you compile a custom kernel with exactly the extra driver(s) you need,
without touching GoSD's own trimming decisions or slowing down everyone
else's build.

## Two tiers

- **Stock artifacts (the default, zero Docker).** `gosd build` downloads
  GoSD's own prebuilt, pinned kernels and bootloaders from a GitHub
  `artifacts/vX.Y.Z` release (see `docs/artifacts.md`) and assembles your
  image. This never requires a container runtime, root, or Linux — it works
  on a bare `go install` on macOS or Linux with no other tooling. Nearly
  every GoSD app stays on this path forever.
- **Custom kernel (opt-in, Docker/Podman required).** Declare your extra
  Kconfig fragment and/or device-tree patches in a `gosd-kernel.toml` in
  your project, run `gosd build-kernel` once to produce a kernel with them
  compiled in, and point `gosd build --artifacts-dir` at the result. This is
  an explicit, opt-in exception to GoSD's usual "no build step needs
  Docker" rule (see `CLAUDE.md`'s locked decisions) — `gosd build` itself
  still never requires a container runtime; only `gosd build-kernel` does,
  and it says so in its own `--help` text and errors.

Both tiers produce an ordinary flat artifact directory
(`kernel8.img`/`Image`, a DTB where the board has one, `kernel.config`,
`source.json`) — `gosd build --artifacts-dir <dir>` doesn't know or care
whether that directory came from a GitHub release download or a local
`gosd build-kernel` run.

## Quickstart

```sh
# In your project, next to a gosd-kernel.toml (see below):
gosd build-kernel --board=pi-zero-2w -o ./gosd-artifacts

# Then build your image from those artifacts instead of a published release:
gosd build . --board pi-zero-2w --artifacts-dir ./gosd-artifacts -o hello.img
```

`gosd build-kernel` requires a local Docker or Podman daemon running; it
drives it directly (`internal/container`), auto-detecting whichever one it
finds unless `--builder` or `gosd-kernel.toml`'s `[kernel].builder` says
otherwise. A kernel build takes 20–120 minutes depending on the board and
your machine; run it backgrounded and let it finish rather than babysitting
a foreground shell.

## Worked example: a USB DVB-T tuner on the Pi Zero 2W (proven)

The Pi Zero 2W's stock kernel fragment
(`build/boards/pi-zero-2w/kernel.fragment`) disables the whole media
subsystem (`# CONFIG_MEDIA_SUPPORT is not set`) — no sound/video/camera
means a smaller, faster-booting kernel for the apps that don't need it. If
your app reads from a USB DVB-T stick (the ubiquitous RTL2832U-based
sticks: RTL2838UHIDIR and clones), you need it back.

`gosd-kernel.toml` in your project root:

```toml
[kernel.pi-zero-2w]
fragment = "dvb.fragment"
```

`dvb.fragment`, next to it:

```
CONFIG_MEDIA_SUPPORT=y
CONFIG_MEDIA_DIGITAL_TV_SUPPORT=y
CONFIG_MEDIA_USB_SUPPORT=y
CONFIG_DVB_CORE=y
CONFIG_DVB_USB_V2=y
CONFIG_DVB_USB_RTL28XXU=y
CONFIG_DVB_RTL2830=y
CONFIG_DVB_RTL2832=y
CONFIG_I2C_MUX=y

# CONFIG_VIDEO_RP1_CFE is not set
# CONFIG_VIDEO_RP1_CFE_DOWNSTREAM is not set
```

### The known wrinkle, and why the last two lines matter

Naively enabling `CONFIG_MEDIA_SUPPORT=y` on the raspberrypi/linux tree used
for this board is not enough on its own — it fails the build. That tree's
`bcm2711_defconfig` sets `CONFIG_EXPERT=y`, which disables the media
subsystem's "filter" menu and so defaults *every* media sub-option,
including `CONFIG_MEDIA_PLATFORM_SUPPORT`, to `y` the moment
`CONFIG_MEDIA_SUPPORT` is on. That in turn default-enables **both** of
raspberrypi/linux's CSI camera front-end drivers —
`drivers/media/platform/raspberrypi/rp1-cfe` and its in-tree
replacement `rp1_cfe`, shipped side by side upstream — and since GoSD
kernels are monolithic (`CONFIG_MODULES` is always off), both get promoted
from the defconfig's `=m` to a built-in `=y` by `make olddefconfig`. Built
in together, they collide at link time: both non-statically define
`dphy_start`/`dphy_stop`/`dphy_probe`, so the final `vmlinux` link fails
with multiple-definition errors.

A DVB-T stick needs neither camera front end, so the fix is to disable both
explicitly (the fragment's last two lines above) rather than fight the
default cascade. This is a real upstream raspberrypi/linux quirk, not a
GoSD bug — if you hit the same collision enabling other
`CONFIG_MEDIA_PLATFORM_SUPPORT`-gated drivers, the same two lines are the
fix, or you'll need to pick one of the two CFE drivers explicitly if you
actually want CSI camera support alongside your driver.

**This exact fragment was built end to end** via `gosd build-kernel
--board=pi-zero-2w` against a real Docker daemon: the build completed
cleanly, and the resulting `kernel.config` carries every symbol above as
`=y`, with both `VIDEO_RP1_CFE*` symbols confirmed absent. Like every board
in GoSD today, it has not been booted on physical Pi Zero 2W hardware yet
(no board has — see `COMPATIBILITY.md`); it has only been verified as a
container build producing a well-formed `kernel.config` and kernel image,
not booted or exercised against a real DVB-T stick.

### Runtime: firmware and the resulting device node

RTL2832U-based sticks need a firmware blob loaded at runtime
(`dvb-usb-rtl28xxu-2.fw` for the RTL2832U demodulator; RTL2831U-based
sticks instead need `dvb-usb-rtl28xxu-1.fw` — check your stick's chipset).
Declare it as a `[[firmware]]` entry in the same `gosd-kernel.toml`:

```toml
[[firmware]]
url    = "https://example.com/dvb-usb-rtl28xxu-2.fw"
sha256 = "<sha256 of the file you fetched>"
dest   = "dvb-usb-rtl28xxu-2.fw"
```

`gosd build --kernel-config gosd-kernel.toml` (or a `gosd-kernel.toml` left
in the working directory, discovered the same way `build-kernel --config`
finds it) fetches and sha256-verifies this the same way board WiFi firmware
is fetched, and places it at `/lib/firmware/dvb-usb-rtl28xxu-2.fw` in the
image's initramfs, alongside the board's own firmware. Never re-hosted by
GoSD, per the project's third-party-blob policy — you point at wherever you
sourced the blob (e.g. the `linux-firmware` project) and pin its hash.

At runtime, plugging the stick into the Pi Zero 2W's USB OTG port (in host
mode) should bring up `/dev/dvb/adapter0/` with `frontend0`, `demux0`, and
`dvr0` nodes, the same as on any other Linux DVB stack. This runtime
behavior has not been hardware-verified (no GoSD board has had any hardware
bring-up yet); it follows directly from the kernel driver and firmware
being present and matches upstream `dvb-usb-rtl28xxu` behavior on any other
Linux kernel with these same options.

## `gosd-kernel.toml` reference

```toml
[kernel]
# Optional. Must equal this gosd's pinned artifacts release (internal/artifacts.Version)
# when set — cross-version kernel builds aren't supported yet. Recorded for your own
# bookkeeping; not currently threaded into source.json (see bean gosd-hkp7's implementation
# notes for why).
based-on = "v0.4.0"
# Optional. "docker" or "podman". Omit to auto-detect; --builder on the CLI wins over this.
builder  = "docker"

[kernel.<board-id>]              # one section per board you customize, keyed by board ID
fragment = "path/to.fragment"    # Kconfig fragment, merged AFTER GoSD's own fragment
patches  = ["patches/*.patch"]   # device-tree patch paths/globs, applied AFTER GoSD's own

[[firmware]]                     # zero or more: runtime firmware blobs your driver needs
url    = "https://example.com/blob.fw"
sha256 = "<64-char lowercase hex sha256>"
dest   = "vendor/blob.fw"        # relative path under /lib/firmware in the initramfs
```

Every key is validated strictly: an unrecognized key anywhere in the file —
even nested inside a board section or a firmware entry — is an error naming
the offending key, since (unlike `gosd.toml`, the end-user-facing runtime
config) this file is a developer-authored build input where a silent typo
should fail loudly rather than silently do nothing.

`[[module]]` — building an out-of-tree loadable `.ko` — is reserved but not
implemented: GoSD kernels are monolithic today (`CONFIG_MODULES` always
off), and whether that ever changes is tracked as a standalone decision in
bean `gosd-2k9p`, deliberately not part of this feature. Compiling a driver
in via `fragment` is the only supported path until (if ever) that decision
changes.

### Overlay semantics

Your fragment is merged with `scripts/kconfig/merge_config.sh -m` **after**
GoSD's own board fragment has already been merged, so a line in your
fragment always wins if it conflicts with GoSD's. Device-tree patches from
`patches` are applied, in sorted-glob order, after every one of GoSD's own
patches. A single `make olddefconfig` runs once, after both fragments are
merged and both patch sets applied — then GoSD's own required-`=y`
assertions for that board are checked against the *result*: you can add to
or override individual options, but you cannot silently drop a symbol GoSD
requires (the build fails, naming the missing symbol) — see
`build/boards/*/kernel.fragment` and the Rockchip boards' `RequiredY` lists
in `internal/kernelspec` for what's asserted per board.

### Caching

Kernel builds are content-addressed: the cache key covers the kernel
ref, the container image, GoSD's own fragment/patches, and your overlay's
fragment/patches. An unchanged input set skips the container run entirely.
Cache entries live under a durable GoSD-managed directory under your home
(not a system temp or evictable-cache location, so a long-running build's
bind mounts and cached outputs survive OS cache purges) — see
`internal/kernelbuild`'s package doc comment for the exact, current path;
it isn't repeated here since it's changed once already as the design
firmed up and a stale path in prose would go stale silently.

### GPL provenance

Every `gosd build-kernel` output directory includes a `source.json`
recording the exact upstream repo and commit/tag the kernel was built from
(and, for boards with one, the config path) — the same provenance shape
stock artifact releases carry in their `manifest.json` for GPL compliance.
If you distribute images built from a custom kernel, you carry the same
GPL source-availability obligations GoSD already handles for you on the
stock artifacts path: keep `source.json` (or the info in it) available
alongside anything you ship.

## Supported hosts

`gosd build-kernel` runs on the same hosts the rest of the CLI supports —
macOS and Linux (amd64/arm64) — with either Docker Desktop or Podman
installed and its daemon/machine running. Windows is untested, matching the
rest of the CLI's "best-effort, don't break gratuitously" stance.
