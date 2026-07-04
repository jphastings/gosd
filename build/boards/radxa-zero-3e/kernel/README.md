# Radxa Zero 3E kernel

Trimmed, module-free mainline arm64 kernel for the Radxa Zero 3E (Rockchip
RK3566). Produces `Image` and the in-tree device tree blob
`rk3566-radxa-zero-3e.dtb`.

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
- `out/rk3566-radxa-zero-3e.dtb` — the device tree blob
- `out/kernel.config` — the full `.config` actually used for that build, for
  comparison against the committed `kernel.config`

## Source and configuration

- Kernel source: mainline stable (`git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git`),
  pinned to the "longterm" (LTS) tag recorded at the top of `build.sh`.
- `kernel-fragment.config` — the hand-maintained fragment of required options
  (SoC, storage, Ethernet, USB, peripherals, and cuts), merged onto
  `make ARCH=arm64 defconfig` via `scripts/kconfig/merge_config.sh`.
- `kernel.config` — the full generated `.config` from the last known-good
  build (header records the source tag/repo/generation method). This is what
  ships in release manifests for GPL compliance; it is not itself fed back
  into the build — `build.sh` always regenerates from `defconfig` + the
  fragment so the build stays reproducible from source.

`docker-build.sh` asserts that the bean's required `CONFIG_*` options are
still set after `make olddefconfig` resolves dependencies, and fails loudly
if trimming or a kernel version bump silently dropped one.

## Updating the pinned kernel version

Bump `KERNEL_TAG` in `build.sh` to a newer mainline "longterm" tag (see
<https://www.kernel.org/releases.json>, look for `"moniker": "longterm"`),
rerun `./build.sh`, then copy `out/kernel.config` over the committed
`kernel.config` and commit both alongside the version bump.

## Known limitations

This has been build-tested but **not boot-tested on hardware** — that is
tracked separately in the bring-up task. Do not treat a clean build as proof
the board boots.
