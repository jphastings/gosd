# Radxa Zero 3E kernel

Trimmed, module-free mainline arm64 kernel for the Radxa Zero 3E (Rockchip
RK3566). Produces `Image` and the in-tree device tree blob
`rk3566-radxa-zero-3e.dtb`.

## Building

```sh
go run ./cmd/gosd build-kernel --board radxa-zero-3e -o out/
```

Requires only Docker. `gosd build-kernel` (bean gosd-07fl) drives everything
— cross toolchain install, kernel source clone, config merge, and compile —
from `internal/kernelspec`'s declarative spec, inside a
`docker.io/library/debian:bookworm` container using the
`aarch64-linux-gnu-` cross prefix, so it produces identical output on an
arm64 host (e.g. Apple Silicon, native container) or an amd64 CI runner
(true cross-compilation). No local kernel source checkout or toolchain
install is needed on the host; there is no board-specific shell script
anymore — `.github/workflows/build-artifacts.yml`'s `radxa-zero-3e-kernel`
job runs the exact same command CI-side.

Outputs land in `out/` (gitignored):

- `out/Image` — the kernel image
- `out/rk3566-radxa-zero-3e.dtb` — the device tree blob
- `out/kernel.config` — the full `.config` actually used for that build, for
  comparison against the committed `kernel.config`
- `out/source.json` — upstream repo/commit and config path, for GPL
  provenance

## Source and configuration

- Kernel source: mainline stable (`git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git`),
  pinned to the "longterm" (LTS) tag recorded in `internal/kernelspec.go`
  (`fleetKernelTag`).
- `kernel-fragment.config` — the hand-maintained fragment of required options
  (SoC, storage, Ethernet, USB, peripherals, and cuts), merged onto
  `make ARCH=arm64 defconfig` via `scripts/kconfig/merge_config.sh`.
- `kernel.config` — the full generated `.config` from the last known-good
  build (header records the source tag/repo/generation method). This is what
  ships in release manifests for GPL compliance; it is not itself fed back
  into the build — `gosd build-kernel` always regenerates from `defconfig` +
  the fragment so the build stays reproducible from source.

`internal/kernelspec.go`'s `RequiredY` list asserts that the bean's required
`CONFIG_*` options are still set after `make olddefconfig` resolves
dependencies, and fails loudly if trimming or a kernel version bump silently
dropped one (formerly `docker-build.sh`'s job, before bean gosd-07fl retired
that script).

## Device-tree patches

`patches/` holds GoSD-authored patches applied (via `patch -p1`) to the
cloned kernel tree right after checkout, before configuring or building.
Mainline's `rk3566-radxa-zero-3e.dts` doesn't enable every peripheral GoSD
wants on by default; rather than fork-and-maintain the whole DTS, each patch
is a small, targeted diff with a comment explaining why it exists.

- `0001-enable-header-i2c3.patch` — enables `i2c3` (`status = "okay"`,
  `pinctrl-0 = <&i2c3m0_xfer>`), which mainline leaves disabled. This is the
  bus wired to the 40-pin header's physical pins 3/5 (GPIO1_A0/A1 — the same
  header position as a Raspberry Pi's GPIO2/3 I2C pins), confirmed against
  Radxa's own schematic and pinout docs. It already enumerates as
  `/dev/i2c-3`: `rk356x-base.dtsi` pre-aliases every `i2cN` node to its own
  number at the SoC level, so no alias addition was needed here (contrast
  with the NanoPi Zero2's patch, where it was). See bean `gosd-85pt`.

This *is* a kernel-pipeline change: a real rebuild via `gosd build-kernel`
(Docker) regenerates `rk3566-radxa-zero-3e.dtb` with `i2c3` enabled. GoSD's committed
artifact releases only ship what's actually compiled (see the repo root
`CLAUDE.md`'s no-rehosting policy), so **this board's DTB artifact needs a
new artifacts release** (`artifacts/vX.Y.Z` tag bump) before real `gosd
build` runs (not using `--artifacts-dir`) pick up the change — the same
tag-then-bump dance as v0.2.0.

If a future U-Boot bump adds `CONFIG_OF_LIBFDT_OVERLAY` / distro-boot
overlay support (checked and absent as of the pinned `v2026.04` tag — see
`../uboot/README.md`), consider migrating this to a `.dtbo` overlay applied
via extlinux's `fdtoverlays` instead, so peripheral toggles stop requiring a
full kernel rebuild.

## Updating the pinned kernel version

Bump `fleetKernelTag` in `internal/kernelspec.go` to a newer mainline
"longterm" tag (see <https://www.kernel.org/releases.json>, look for
`"moniker": "longterm"` — this same constant is shared with nanopi-zero2 and
qemu-virt, so bump all three together), rerun the build command above, then
copy `out/kernel.config` over the committed `kernel.config` and commit both
alongside the version bump. Re-check that `patches/*.patch` still applies
cleanly against the new tag's DTS — a DTS restructure upstream could require
regenerating them.

## Known limitations

This has been build-tested but **not boot-tested on hardware** — that is
tracked separately in the bring-up task. Do not treat a clean build as proof
the board boots.
