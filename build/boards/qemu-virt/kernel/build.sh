#!/usr/bin/env bash
# Builds the trimmed mainline arm64 kernel for the qemu-virt board (an
# internal-only profile for CI/local testing under `qemu-system-aarch64 -M
# virt` — see bean gosd-5wm0 and the "qemu-virt board" locked decision in
# CLAUDE.md; this board is EXCLUDED from default all-boards builds and
# end-user docs).
#
# Runs entirely inside docker.io/library/debian:bookworm using the
# aarch64-linux-gnu- cross toolchain, so it produces the same output whether
# run on an arm64 host or an amd64 CI runner — no reliance on the host's own
# compiler or on QEMU-emulated arm64 containers.
set -euo pipefail

# Pinned to the same mainline stable "longterm" (LTS) tag as the Radxa Zero
# 3E kernel build, so the two boards' config fragments stay diff-able
# against a single kernel source tree. See
# build/boards/radxa-zero-3e/kernel/build.sh.
KERNEL_TAG="v6.18.37"
KERNEL_REPO="https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${SCRIPT_DIR}/out"

mkdir -p "${OUT_DIR}"

docker run --rm \
  -e KERNEL_TAG="${KERNEL_TAG}" \
  -e KERNEL_REPO="${KERNEL_REPO}" \
  -v "${SCRIPT_DIR}:/work:ro" \
  -v "${OUT_DIR}:/out" \
  -w /build \
  docker.io/library/debian:bookworm \
  bash /work/docker-build.sh

echo "Build complete. Outputs in ${OUT_DIR}:"
ls -la "${OUT_DIR}"
