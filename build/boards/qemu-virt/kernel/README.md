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
go run ./cmd/gosd build-kernel --board qemu-virt -o out/
```

Requires only Docker. `gosd build-kernel` (bean gosd-07fl) drives everything
— cross toolchain install, kernel source clone, config merge, and compile —
from `internal/kernelspec`'s declarative spec, inside a
`docker.io/library/debian:bookworm` container using the
`aarch64-linux-gnu-` cross prefix, so it produces identical output on an
arm64 host (e.g. Apple Silicon, native container) or an amd64 CI runner
(true cross-compilation). No local kernel source checkout or toolchain
install is needed on the host; there is no board-specific shell script
anymore — `.github/workflows/build-artifacts.yml`'s `qemu-virt-kernel` job
runs the exact same command CI-side.

Outputs land in `out/` (gitignored):

- `out/Image` — the kernel image
- `out/kernel.config` — the full `.config` actually used for that build, for
  comparison against the committed `kernel.config`
- `out/source.json` — upstream repo/commit and config path, for GPL
  provenance

## Source and configuration

- Kernel source: mainline stable (`git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git`),
  pinned to the same "longterm" (LTS) tag as radxa-zero-3e's kernel (see
  `fleetKernelTag` in `internal/kernelspec.go`), so the two boards' fragments
  stay diff-able against one kernel source tree.
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
  back into the build — `gosd build-kernel` always regenerates from
  `defconfig` + the fragment so the build stays reproducible from source.

`internal/kernelspec.go`'s `RequiredY`/`ForbiddenY` lists assert that every
required `CONFIG_*` option is still set after `make olddefconfig` resolves
dependencies, and separately assert the cut real-hardware options stayed off
— both fail loudly if trimming or a kernel version bump silently changed
either list (formerly `docker-build.sh`'s job, before bean gosd-07fl retired
that script).

## Updating the pinned kernel version

Bump `fleetKernelTag` in `internal/kernelspec.go` — normally in lockstep
with radxa-zero-3e's and nanopi-zero2's, since all three pin the same
mainline "longterm" tag (see <https://www.kernel.org/releases.json>, look
for `"moniker": "longterm"`) — rerun the build command above, then copy
`out/kernel.config` over the committed `kernel.config` and commit both
alongside the version bump.

## Boot validation

Unlike the hardware boards, this kernel's whole reason for existing is to be
booted in CI, so it's boot-tested as part of building it — not deferred to a
separate bring-up task. See the bean (gosd-5wm0) for the exact
`qemu-system-aarch64` invocation used to confirm a minimal initramfs's
marker output reaches the serial console, and timing notes for running QEMU
TCG emulation inside Docker on an Apple Silicon host.

The full boot-to-HTTP smoke test (a real GoSD app image, not just a marker
init) is a separate task: gosd-27lz.
