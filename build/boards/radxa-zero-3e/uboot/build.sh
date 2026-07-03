#!/usr/bin/env bash
# Builds mainline U-Boot (idbloader.img + u-boot.itb) for the Radxa Zero 3E,
# inside Docker, and writes the artifacts to ./out/.
#
# Usage: ./build.sh
#
# Pinned inputs (edit here / in manifest.json to bump versions):
#   - U-Boot release tag: UBOOT_TAG below (mainline, >= v2025.01).
#   - rkbin repo commit + blob paths + sha256: ../manifest.json.
set -euo pipefail

UBOOT_TAG="v2026.04"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST="${SCRIPT_DIR}/../manifest.json"
OUT_DIR="${SCRIPT_DIR}/out"
IMAGE_TAG="gosd-radxa-zero-3e-uboot-build"

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker is required to build U-Boot for this board; install Docker and try again" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is required to read ${MANIFEST}; install jq (e.g. 'brew install jq') and try again" >&2
  exit 1
fi

if [ ! -f "$MANIFEST" ]; then
  echo "error: manifest not found at ${MANIFEST}; this file pins the rkbin blobs and must exist" >&2
  exit 1
fi

RKBIN_COMMIT=$(jq -r '.rkbin.commit' "$MANIFEST")
RKBIN_DDR_PATH=$(jq -r '.rkbin.blobs.ddr_tpl.path' "$MANIFEST")
RKBIN_DDR_SHA256=$(jq -r '.rkbin.blobs.ddr_tpl.sha256' "$MANIFEST")
RKBIN_BL31_PATH=$(jq -r '.rkbin.blobs.bl31.path' "$MANIFEST")
RKBIN_BL31_SHA256=$(jq -r '.rkbin.blobs.bl31.sha256' "$MANIFEST")

for name in RKBIN_COMMIT RKBIN_DDR_PATH RKBIN_DDR_SHA256 RKBIN_BL31_PATH RKBIN_BL31_SHA256; do
  if [ -z "${!name}" ] || [ "${!name}" = "null" ]; then
    echo "error: ${MANIFEST} is missing a value for ${name}; check the manifest's rkbin section" >&2
    exit 1
  fi
done

echo "Building U-Boot ${UBOOT_TAG} for radxa-zero-3e (rkbin @ ${RKBIN_COMMIT:0:12})..."

docker build \
  --target artifacts \
  --tag "$IMAGE_TAG" \
  --build-arg "UBOOT_TAG=${UBOOT_TAG}" \
  --build-arg "RKBIN_COMMIT=${RKBIN_COMMIT}" \
  --build-arg "RKBIN_DDR_PATH=${RKBIN_DDR_PATH}" \
  --build-arg "RKBIN_DDR_SHA256=${RKBIN_DDR_SHA256}" \
  --build-arg "RKBIN_BL31_PATH=${RKBIN_BL31_PATH}" \
  --build-arg "RKBIN_BL31_SHA256=${RKBIN_BL31_SHA256}" \
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
