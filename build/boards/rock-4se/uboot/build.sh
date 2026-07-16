#!/usr/bin/env bash
# Builds mainline U-Boot (idbloader.img + u-boot.itb) for the Radxa ROCK 4SE,
# inside Docker, and writes the artifacts to ./out/.
#
# Usage: ./build.sh
#
# Pinned inputs (edit here / in manifest.json to bump versions):
#   - U-Boot release tag: UBOOT_TAG below (mainline, >= v2025.01).
#   - TF-A repo + tag + peeled commit: ../manifest.json (compiled from
#     source -- no rkbin blobs on RK3399, see this directory's README).
set -euo pipefail

UBOOT_TAG="v2026.04"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST="${SCRIPT_DIR}/../manifest.json"
OUT_DIR="${SCRIPT_DIR}/out"
IMAGE_TAG="gosd-rock-4se-uboot-build"

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
TFA_TAG=$(jq -r '.tfa.tag' "$MANIFEST")
TFA_COMMIT=$(jq -r '.tfa.commit' "$MANIFEST")

for name in TFA_REPO TFA_TAG TFA_COMMIT; do
  if [ -z "${!name}" ] || [ "${!name}" = "null" ]; then
    echo "error: ${MANIFEST} is missing a value for ${name}; check the manifest's tfa section" >&2
    exit 1
  fi
done

echo "Building U-Boot ${UBOOT_TAG} for rock-4se (TF-A ${TFA_TAG} @ ${TFA_COMMIT:0:12})..."

docker build \
  --target artifacts \
  --tag "$IMAGE_TAG" \
  --build-arg "UBOOT_TAG=${UBOOT_TAG}" \
  --build-arg "TFA_REPO=${TFA_REPO}" \
  --build-arg "TFA_TAG=${TFA_TAG}" \
  --build-arg "TFA_COMMIT=${TFA_COMMIT}" \
  "$SCRIPT_DIR"

mkdir -p "$OUT_DIR"
# The artifacts stage is FROM scratch (no shell, no CMD), so `docker create`
# needs a placeholder command argument to satisfy container config
# validation -- it's never executed, we only use the container to `docker cp`
# out of its filesystem.
CONTAINER_ID=$(docker create "$IMAGE_TAG" placeholder)
trap 'docker rm -f "$CONTAINER_ID" >/dev/null 2>&1 || true' EXIT

docker cp "${CONTAINER_ID}:/idbloader.img" "${OUT_DIR}/idbloader.img"
docker cp "${CONTAINER_ID}:/u-boot.itb" "${OUT_DIR}/u-boot.itb"

echo "Done. Artifacts written to ${OUT_DIR}/idbloader.img and ${OUT_DIR}/u-boot.itb"
