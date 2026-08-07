#!/usr/bin/env bash
# Builds mainline U-Boot (u-boot-sunxi-with-spl.bin) for the Radxa Cubie A5E,
# inside Docker, and writes the artifact to ./out/.
#
# Usage: ./build.sh
#
# Pinned inputs (edit here / in manifest.json to bump versions):
#   - U-Boot release tag: UBOOT_TAG below (mainline, >= v2026.04).
#   - TF-A repo + branch + commit: ../manifest.json (compiled from source --
#     no rkbin-style blobs on the A527, see this directory's README). The
#     commit is authoritative; the branch is informational only.
set -euo pipefail

UBOOT_TAG="v2026.04"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST="${SCRIPT_DIR}/../manifest.json"
OUT_DIR="${SCRIPT_DIR}/out"
IMAGE_TAG="gosd-cubie-a5e-uboot-build"

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker is required to build U-Boot for this board; install Docker and try again" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is required to read ${MANIFEST}; install jq (e.g. 'brew install jq') and try again" >&2
  exit 1
fi

if [ ! -f "$MANIFEST" ]; then
  echo "error: manifest not found at ${MANIFEST}; this file pins the TF-A source and must exist" >&2
  exit 1
fi

TFA_REPO=$(jq -r '.tfa.repo' "$MANIFEST")
TFA_BRANCH=$(jq -r '.tfa.branch' "$MANIFEST")
TFA_COMMIT=$(jq -r '.tfa.commit' "$MANIFEST")

for name in TFA_REPO TFA_BRANCH TFA_COMMIT; do
  if [ -z "${!name}" ] || [ "${!name}" = "null" ]; then
    echo "error: ${MANIFEST} is missing a value for ${name}; check the manifest's tfa section" >&2
    exit 1
  fi
done

echo "Building U-Boot ${UBOOT_TAG} for cubie-a5e (TF-A ${TFA_REPO} @ ${TFA_COMMIT:0:12}, branch ${TFA_BRANCH} is informational only)..."

docker build \
  --target artifacts \
  --tag "$IMAGE_TAG" \
  --build-arg "UBOOT_TAG=${UBOOT_TAG}" \
  --build-arg "TFA_REPO=${TFA_REPO}" \
  --build-arg "TFA_COMMIT=${TFA_COMMIT}" \
  "$SCRIPT_DIR"

mkdir -p "$OUT_DIR"
# The artifacts stage is FROM scratch (no shell, no CMD), so `docker create`
# needs a placeholder command argument to satisfy container config
# validation -- it's never executed, we only use the container to `docker cp`
# out of its filesystem.
CONTAINER_ID=$(docker create "$IMAGE_TAG" placeholder)
trap 'docker rm -f "$CONTAINER_ID" >/dev/null 2>&1 || true' EXIT

docker cp "${CONTAINER_ID}:/u-boot-sunxi-with-spl.bin" "${OUT_DIR}/u-boot-sunxi-with-spl.bin"

echo "Done. Artifact written to ${OUT_DIR}/u-boot-sunxi-with-spl.bin"
