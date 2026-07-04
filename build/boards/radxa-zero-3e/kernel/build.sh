#!/usr/bin/env bash
# Builds the trimmed mainline arm64 kernel for the Radxa Zero 3E (RK3566).
#
# Runs entirely inside docker.io/library/debian:bookworm using the
# aarch64-linux-gnu- cross toolchain, so it produces the same output whether
# run on an arm64 host or an amd64 CI runner — no reliance on the host's own
# compiler or on QEMU-emulated arm64 containers.
set -euo pipefail

# Pinned mainline stable "longterm" (LTS) release, >= 6.12, per
# https://www.kernel.org/releases.json at the time this was written.
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
