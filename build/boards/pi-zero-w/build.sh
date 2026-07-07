#!/usr/bin/env bash
# Builds the trimmed Pi Zero W kernel (gosd-s7fk, epic gosd-ajpz) inside a
# Debian bookworm Docker container, cross-compiling 32-bit arm via
# crossbuild-essential-armhf (arm-linux-gnueabihf-gcc). Unlike the
# pi-zero-2w/radxa-zero-3e arm64 builds, this is a TRUE cross-compile even on
# an arm64 host: the Zero W's ARM1176JZF-S is armv6, so the armv7-tuned
# gnueabihf toolchain must be told to target armv6 by the kernel's own
# -march/-mtune (Kconfig CONFIG_ARM_ARCH_6 etc via bcmrpi_defconfig), not by
# the toolchain's default tuning. This is standard practice for building
# upstream/raspberrypi rpi armv6 kernels; see README.md for detail.
#
# Outputs, written to out/ next to this script:
#   kernel.img                    (renamed arch/arm/boot/zImage)
#   bcm2835-rpi-zero-w.dtb
#   generated-kernel.config       (the .config the build actually used,
#                                  for comparison against kernel.config)
set -euo pipefail

BOARD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="$BOARD_DIR/out"

REPO_URL="https://github.com/raspberrypi/linux.git"

# Pinned to the SAME commit as build/boards/pi-zero-2w/build.sh, for fleet
# consistency across the two Broadcom boards (see that file for why this
# particular commit on rpi-6.18.y was chosen).
PINNED_COMMIT="63598c83153e19b1f99067ab6df7409de2c111f8"
COMMIT_DATE="2026-07-01T10:23:21Z"

# bcmrpi_defconfig is raspberrypi/linux's armv6 defconfig, covering the
# original BCM2835 family (Pi 1, Pi Zero, Pi Zero W) via CONFIG_ARCH_BCM2835=y
# in arch/arm/configs. Confirmed present at the pinned commit, alongside
# arch/arm/boot/dts/broadcom/bcm2835-rpi-zero-w.dts (the DTS path moved under
# broadcom/ in the same reorg that affected the arm64 tree; see
# pi-zero-2w/README.md's note on the equivalent arm64 move).
DEFCONFIG="bcmrpi_defconfig"
DTB_NAME="bcm2835-rpi-zero-w.dtb"

DOCKER_IMAGE="docker.io/library/debian:bookworm"

mkdir -p "$OUT_DIR"

INNER_SCRIPT="$(mktemp)"
trap 'rm -f "$INNER_SCRIPT"' EXIT

cat >"$INNER_SCRIPT" <<'INNER'
set -euo pipefail
REPO_URL="$1"
PINNED_COMMIT="$2"
COMMIT_DATE="$3"
DEFCONFIG="$4"
DTB_NAME="$5"

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
# build-essential: native host cc/gcc for HOSTCC tools (e.g.
# scripts/basic/fixdep) — see pi-zero-2w/build.sh for why this can't be
# dropped even though crossbuild-essential-armhf also pulls in a native
# toolchain on an arm64 host. crossbuild-essential-armhf provides the
# arm-linux-gnueabihf-* CROSS tools this is a genuine cross-compile with,
# regardless of host arch (arm64 or amd64 CI runners).
apt-get install -y -qq --no-install-recommends \
  ca-certificates git make bc bison flex libssl-dev libelf-dev \
  python3 rsync cpio kmod build-essential crossbuild-essential-armhf >/dev/null

mkdir -p /build/linux
cd /build/linux
git init -q
git remote add origin "$REPO_URL"
git fetch -q --depth 1 origin "$PINNED_COMMIT"
git checkout -q FETCH_HEAD

export ARCH=arm
export CROSS_COMPILE=arm-linux-gnueabihf-
# Reproducible build metadata, tied to the pinned commit rather than
# wall-clock time or this container's hostname/user.
export KBUILD_BUILD_TIMESTAMP="$COMMIT_DATE"
export KBUILD_BUILD_USER="gosd"
export KBUILD_BUILD_HOST="gosd-ci"

make "$DEFCONFIG"
scripts/kconfig/merge_config.sh -m .config /board/kernel.fragment
make olddefconfig

cp .config /out/generated-kernel.config

make -j"$(nproc)" zImage dtbs

install -m 0644 arch/arm/boot/zImage /out/kernel.img
install -m 0644 "arch/arm/boot/dts/broadcom/$DTB_NAME" /out/
INNER

docker run --rm \
  -v "$BOARD_DIR:/board:ro" \
  -v "$OUT_DIR:/out" \
  -v "$INNER_SCRIPT:/build-inner.sh:ro" \
  "$DOCKER_IMAGE" \
  bash /build-inner.sh "$REPO_URL" "$PINNED_COMMIT" "$COMMIT_DATE" "$DEFCONFIG" "$DTB_NAME"

echo "Wrote $OUT_DIR/kernel.img and $OUT_DIR/$DTB_NAME"
