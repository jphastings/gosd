#!/usr/bin/env bash
# Builds the trimmed Pi Zero 2W kernel (gosd-70b2) inside a Debian
# bookworm Docker container, cross-compiling arm64 via
# crossbuild-essential-arm64 (aarch64-linux-gnu-gcc). This works
# unchanged whether the host is arm64 (the "cross" toolchain is
# effectively native) or amd64 (a real cross toolchain, e.g. CI).
#
# Outputs, written to out/ next to this script:
#   kernel8.img                  (renamed arch/arm64/boot/Image)
#   bcm2710-rpi-zero-2-w.dtb
#   generated-kernel.config      (the .config the build actually used,
#                                 for comparison against kernel.config)
set -euo pipefail

BOARD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="$BOARD_DIR/out"

REPO_URL="https://github.com/raspberrypi/linux.git"

# Pinned to the current head of rpi-6.18.y, the latest maintained
# stable branch as of 2026-07-03 (rpi-6.19.y exists but was declared
# EOL on 2026-04-27; rpi-6.18.y is raspberrypi/linux's default branch).
PINNED_COMMIT="63598c83153e19b1f99067ab6df7409de2c111f8"
COMMIT_DATE="2026-07-01T10:23:21Z"

# The bean names bcmrpi3_defconfig as the starting point, but that
# defconfig was deleted upstream (commit 7713244d3baee3493108fb98edd82f5b2042ce48,
# "configs: Delete bcmrpi3_defconfig": "neither used nor supported by
# us") and has been gone from arch/arm64/configs since rpi-6.12.y.
# bcm2711_defconfig is its arm64 successor: it's the one currently-
# maintained defconfig covering BCM2710/2711/2712 boards (Zero 2 W, 3,
# 4, CM4, ...) via CONFIG_ARCH_BCM2835=y, and is what this script
# starts from instead. See kernel.fragment and README.md for detail.
DEFCONFIG="bcm2711_defconfig"
DTB_NAME="bcm2710-rpi-zero-2-w.dtb"

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
# scripts/basic/fixdep). On an arm64 host, crossbuild-essential-arm64 alone
# happens to pull in a native toolchain too, which masked this dependency
# until CI ran on amd64 runners, where crossbuild-essential-arm64 only
# provides the aarch64-linux-gnu-* CROSS tools.
apt-get install -y -qq --no-install-recommends \
  ca-certificates git make bc bison flex libssl-dev libelf-dev \
  python3 rsync cpio kmod build-essential crossbuild-essential-arm64 >/dev/null

mkdir -p /build/linux
cd /build/linux
git init -q
git remote add origin "$REPO_URL"
git fetch -q --depth 1 origin "$PINNED_COMMIT"
git checkout -q FETCH_HEAD

export ARCH=arm64
export CROSS_COMPILE=aarch64-linux-gnu-
# Reproducible build metadata, tied to the pinned commit rather than
# wall-clock time or this container's hostname/user.
export KBUILD_BUILD_TIMESTAMP="$COMMIT_DATE"
export KBUILD_BUILD_USER="gosd"
export KBUILD_BUILD_HOST="gosd-ci"

make "$DEFCONFIG"
scripts/kconfig/merge_config.sh -m .config /board/kernel.fragment
make olddefconfig

cp .config /out/generated-kernel.config

make -j"$(nproc)" Image dtbs

install -m 0644 arch/arm64/boot/Image /out/kernel8.img
install -m 0644 "arch/arm64/boot/dts/broadcom/$DTB_NAME" /out/
INNER

docker run --rm \
  -v "$BOARD_DIR:/board:ro" \
  -v "$OUT_DIR:/out" \
  -v "$INNER_SCRIPT:/build-inner.sh:ro" \
  "$DOCKER_IMAGE" \
  bash /build-inner.sh "$REPO_URL" "$PINNED_COMMIT" "$COMMIT_DATE" "$DEFCONFIG" "$DTB_NAME"

echo "Wrote $OUT_DIR/kernel8.img and $OUT_DIR/$DTB_NAME"
