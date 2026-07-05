# qemu-virt kernel

Trimmed, module-free mainline arm64 kernel for the **qemu-virt board** — an
internal-only profile for booting real GoSD images under
`qemu-system-aarch64 -M virt` in CI and local development (bean gosd-5wm0,
epic gosd-c54j). It is **not** an end-user board: it's excluded from default
`gosd build` (no `--board`) output and from end-user docs; see the
"qemu-virt board" locked decision in the repo root `CLAUDE.md`.

Produces `Image` only — no device tree blob. `qemu -M virt` synthesizes its
own DTB at boot time, so none is built or shipped here.

## Building

```sh
./build.sh
```

Requires only Docker. The script runs everything — cross toolchain install,
kernel source clone, config merge, and compile — inside a
`docker.io/library/debian:bookworm` container using the
`aarch64-linux-gnu-` cross prefix, so it produces identical output on an
arm64 host (e.g. Apple Silicon, native container) or an amd64 CI runner
(true cross-compilation). No local kernel source checkout or toolchain
install is needed on the host.

Outputs land in `out/` (gitignored):

- `out/Image` — the kernel image
- `out/kernel.config` — the full `.config` actually used for that build, for
  comparison against the committed `kernel.config`

## Source and configuration

- Kernel source: mainline stable (`git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git`),
  pinned to the same "longterm" (LTS) tag as
  `build/boards/radxa-zero-3e/kernel/build.sh` (see `KERNEL_TAG` at the top
  of `build.sh`), so the two boards' fragments stay diff-able against one
  kernel source tree.
- `kernel-fragment.config` — the hand-maintained fragment of required
  options: the standard GoSD baseline (initramfs RD_ZSTD, VFAT_FS+NLS,
  devtmpfs, IPv4/6, AF_PACKET), virtio blk/net over both PCI and MMIO
  transports, PL011 console (`ttyAMA0`), PL031 RTC, `CONFIG_MODULES=n`, and
  explicit cuts of every real-hardware driver (Rockchip, Broadcom, WiFi,
  sound, media, DRM). Merged onto `make ARCH=arm64 defconfig` via
  `scripts/kconfig/merge_config.sh`.
- `kernel.config` — the full generated `.config` from the last known-good
  build (header records the source tag/repo/generation method). This is
  what ships in artifact manifests for GPL compliance; it is not itself fed
  back into the build — `build.sh` always regenerates from `defconfig` +
  the fragment so the build stays reproducible from source.

`docker-build.sh` asserts that every required `CONFIG_*` option is still set
after `make olddefconfig` resolves dependencies, and separately asserts the
cut real-hardware options stayed off — both fail loudly if trimming or a
kernel version bump silently changed either list.

## Updating the pinned kernel version

Bump `KERNEL_TAG` in `build.sh` — normally in lockstep with
`build/boards/radxa-zero-3e/kernel/build.sh`'s own `KERNEL_TAG`, since both
pin the same mainline "longterm" tag (see
<https://www.kernel.org/releases.json>, look for `"moniker": "longterm"`) —
rerun `./build.sh`, then copy `out/kernel.config` over the committed
`kernel.config` and commit both alongside the version bump.

## Boot validation

Unlike the hardware boards, this kernel's whole reason for existing is to be
booted in CI, so it's boot-tested as part of building it — not deferred to a
separate bring-up task. See the bean (gosd-5wm0) for the exact
`qemu-system-aarch64` invocation used to confirm a minimal initramfs's
marker output reaches the serial console, and timing notes for running QEMU
TCG emulation inside Docker on an Apple Silicon host.

The full boot-to-HTTP smoke test (a real GoSD app image, not just a marker
init) is a separate task: gosd-27lz.
