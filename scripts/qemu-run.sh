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
# The actual work here - extracting the kernel Image and
# initramfs.cpio.zst a qemu-virt image carries on its FAT boot partition
# (qemu has no bootloader of its own to read them off the partition the
# way real hardware does) and the qemu-system-aarch64 invocation itself -
# lives in internal/qemurun, the same package `gosd run` (cmd/gosd) uses
# to build and boot an image in one step. This script is a thin wrapper
# around internal/cmd/qemuboot so an already-built image (and anything
# already scripted against this file, including CI's qemu-boot job) keeps
# working unchanged.
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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
IMG="$(cd "$(dirname "$IMG")" && pwd)/$(basename "$IMG")"

exec go run -C "${REPO_ROOT}" ./internal/cmd/qemuboot "${IMG}"
