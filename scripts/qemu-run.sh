#!/usr/bin/env bash
# Boots a gosd image built with --board=qemu-virt under
# qemu-system-aarch64, with serial console on stdio so you can watch
# gosd-init and your app's logs live, and the app's HTTP port reachable at
# localhost:8080.
#
# Usage: scripts/qemu-run.sh <path-to-image.img>
#
# Requires qemu-system-aarch64 on PATH:
#   macOS:  brew install qemu
#   Debian/Ubuntu: apt-get install qemu-system-arm
#
# The .img's FAT boot partition carries the kernel Image and
# initramfs.cpio.zst qemu needs for -kernel/-initrd (see
# internal/boards/qemuvirt): qemu has no bootloader of its own to read them
# off the partition itself, unlike real hardware, so this script extracts
# them first via internal/cmd/imgextract (go-diskfs, no root, no mtools).
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <path-to-image.img>" >&2
  exit 1
fi
IMG=$1

if [ ! -f "$IMG" ]; then
  echo "qemu-run.sh: no such image file: $IMG" >&2
  exit 1
fi

if ! command -v qemu-system-aarch64 >/dev/null 2>&1; then
  echo "qemu-run.sh: qemu-system-aarch64 not found on PATH." >&2
  echo "Install it: 'brew install qemu' (macOS) or 'apt-get install qemu-system-arm' (Debian/Ubuntu)." >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
IMG="$(cd "$(dirname "$IMG")" && pwd)/$(basename "$IMG")"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

echo "qemu-run.sh: extracting Image + initramfs.cpio.zst from ${IMG}'s boot partition..." >&2
( cd "${REPO_ROOT}" && go run ./internal/cmd/imgextract "${IMG}" "${WORKDIR}" )

for f in Image initramfs.cpio.zst; do
  if [ ! -f "${WORKDIR}/${f}" ]; then
    echo "qemu-run.sh: ${IMG}'s boot partition has no ${f}; is this a --board=qemu-virt image?" >&2
    exit 1
  fi
done

echo "qemu-run.sh: booting ${IMG} (Ctrl-A X to quit qemu, Ctrl-C to force-kill)." >&2
echo "qemu-run.sh: your app will be reachable at http://localhost:8080 once gosd-init starts it and networking comes up." >&2

exec qemu-system-aarch64 \
  -M virt -cpu cortex-a53 -m 512 \
  -nographic \
  -kernel "${WORKDIR}/Image" \
  -initrd "${WORKDIR}/initramfs.cpio.zst" \
  -append "console=ttyAMA0 gosd.board=qemu-virt" \
  -drive if=none,file="${IMG}",format=raw,id=hd0 \
  -device virtio-blk-pci,drive=hd0,romfile= \
  -netdev user,id=n0,hostfwd=tcp::8080-:80 \
  -device virtio-net-pci,netdev=n0,romfile=
