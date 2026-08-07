#!/usr/bin/env bash
# Regenerates the ext4 golden image checked into
# internal/diskfmt/ext4golden/golden.img.zst: builds e2fsprogs from source
# inside Docker, runs mke2fs with GoSD's pinned parameters, verifies the
# online-resize path in a privileged container, then compresses and stamps
# the checked-in asset + its manifest.json.
#
# Usage: ./build.sh
#
# Pinned inputs (edit here to change the golden image):
#   - e2fsprogs release tag + commit: E2FSPROGS_TAG / E2FSPROGS_COMMIT below.
#   - mke2fs parameters (feature set, block/journal size, fixed UUID/hash
#     seed): GOLDEN_UUID / HASH_SEED / GOLDEN_SIZE_MB / JOURNAL_SIZE_MB
#     below, and the -O/-E flags baked into Dockerfile's mke2fs RUN step.
#     WHY each one was chosen is recorded in ../../internal/diskfmt/ext4golden/README.md,
#     not repeated here.
set -euo pipefail

E2FSPROGS_TAG="v1.47.4"
E2FSPROGS_COMMIT="7ee1d505ef3b37831215f490411f346fe57e9053"

# Fixed, arbitrary placeholders -- not real per-volume identity. gosd-apmv
# stamps a fresh random UUID (and the real label) into the superblock at
# format time; these values only need to be fixed, not secret or unique.
GOLDEN_UUID="4c1a41c8-20b8-4c50-8399-7fae324e8398"
HASH_SEED="da89e13f-1cf4-4015-a4e0-0e9abbd2aabd"
# A fixed epoch (2025-01-01T00:00:00Z), not "now" -- every regen produces
# the same on-disk timestamps regardless of when build.sh actually runs,
# which is what makes the raw image byte-reproducible.
FAKE_TIME="1735689600"

GOLDEN_SIZE_MB="512"
JOURNAL_SIZE_MB="128"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${SCRIPT_DIR}/out"
ASSET_DIR="$(cd "${SCRIPT_DIR}/../../internal/diskfmt/ext4golden" && pwd)"
BUILD_TAG="gosd-ext4-golden-build"
ARTIFACTS_TAG="gosd-ext4-golden-artifacts"

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker is required to build the ext4 golden image; install Docker and try again" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is required to write manifest.json; install jq (e.g. 'brew install jq') and try again" >&2
  exit 1
fi

echo "Building e2fsprogs ${E2FSPROGS_TAG} (@ ${E2FSPROGS_COMMIT:0:12}) and the ext4 golden image..."

docker build \
  --target build \
  --tag "$BUILD_TAG" \
  --build-arg "E2FSPROGS_TAG=${E2FSPROGS_TAG}" \
  --build-arg "E2FSPROGS_COMMIT=${E2FSPROGS_COMMIT}" \
  --build-arg "GOLDEN_UUID=${GOLDEN_UUID}" \
  --build-arg "HASH_SEED=${HASH_SEED}" \
  --build-arg "FAKE_TIME=${FAKE_TIME}" \
  --build-arg "GOLDEN_SIZE_MB=${GOLDEN_SIZE_MB}" \
  --build-arg "JOURNAL_SIZE_MB=${JOURNAL_SIZE_MB}" \
  "$SCRIPT_DIR"

docker build \
  --target artifacts \
  --tag "$ARTIFACTS_TAG" \
  --build-arg "E2FSPROGS_TAG=${E2FSPROGS_TAG}" \
  --build-arg "E2FSPROGS_COMMIT=${E2FSPROGS_COMMIT}" \
  --build-arg "GOLDEN_UUID=${GOLDEN_UUID}" \
  --build-arg "HASH_SEED=${HASH_SEED}" \
  --build-arg "FAKE_TIME=${FAKE_TIME}" \
  --build-arg "GOLDEN_SIZE_MB=${GOLDEN_SIZE_MB}" \
  --build-arg "JOURNAL_SIZE_MB=${JOURNAL_SIZE_MB}" \
  "$SCRIPT_DIR"

mkdir -p "$OUT_DIR"
CONTAINER_ID=$(docker create "$ARTIFACTS_TAG" placeholder)
trap 'docker rm -f "$CONTAINER_ID" >/dev/null 2>&1 || true' EXIT
docker cp "${CONTAINER_ID}:/golden.img" "${OUT_DIR}/golden.img"
docker cp "${CONTAINER_ID}:/golden.dumpe2fs.txt" "${OUT_DIR}/golden.dumpe2fs.txt"
docker rm -f "$CONTAINER_ID" >/dev/null 2>&1 || true
trap - EXIT

echo
echo "Running in-container verification (privileged loop-mount + online resize2fs + fsck)..."
VERIFY_LOG="${OUT_DIR}/verify.log"
VERIFY_RAN=false
GROWTH_CEILING_BYTES=""
if docker run --rm --privileged "$BUILD_TAG" /build/verify.sh 2>&1 | tee "$VERIFY_LOG"; then
  if grep -q "verify: PASSED" "$VERIFY_LOG"; then
    VERIFY_RAN=true
    GROWTH_CEILING_BYTES=$(grep -oE 'VERIFIED_GROWTH_CEILING_BYTES=[0-9]+' "$VERIFY_LOG" | cut -d= -f2)
    echo "Verification PASSED. Growth ceiling proven this run: ${GROWTH_CEILING_BYTES} bytes."
  else
    echo "Verification SKIPPED (see ${VERIFY_LOG} for why) -- runtime verification falls to the qemu-virt smoke test (bean gosd-ucgr)."
  fi
else
  echo "error: in-container verification FAILED -- see ${VERIFY_LOG}" >&2
  exit 1
fi

echo
echo "Compressing with zstd (inside the pinned build container)..."
docker run --rm -v "${OUT_DIR}:/hostout" "$BUILD_TAG" \
  zstd -19 -q -f -o /hostout/golden.img.zst /build/golden.img

RAW_SHA256=$(shasum -a 256 "${OUT_DIR}/golden.img" | cut -d' ' -f1)
COMPRESSED_SHA256=$(shasum -a 256 "${OUT_DIR}/golden.img.zst" | cut -d' ' -f1)
COMPRESSED_SIZE=$(stat -f%z "${OUT_DIR}/golden.img.zst" 2>/dev/null || stat -c%s "${OUT_DIR}/golden.img.zst")
RAW_SIZE=$(stat -f%z "${OUT_DIR}/golden.img" 2>/dev/null || stat -c%s "${OUT_DIR}/golden.img")

cp "${OUT_DIR}/golden.img.zst" "${ASSET_DIR}/golden.img.zst"

GENERATED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
if [ -n "$GROWTH_CEILING_BYTES" ]; then
  growth_arg=(--argjson verifiedGrowthCeilingBytes "$GROWTH_CEILING_BYTES")
else
  growth_arg=(--argjson verifiedGrowthCeilingBytes null)
fi

jq -n \
  --arg e2fsprogsTag "$E2FSPROGS_TAG" \
  --arg e2fsprogsCommit "$E2FSPROGS_COMMIT" \
  --arg goldenUUID "$GOLDEN_UUID" \
  --arg hashSeed "$HASH_SEED" \
  --arg fakeTime "$FAKE_TIME" \
  --argjson goldenSizeMB "$GOLDEN_SIZE_MB" \
  --argjson journalSizeMB "$JOURNAL_SIZE_MB" \
  --argjson rawSize "$RAW_SIZE" \
  --arg rawSHA256 "$RAW_SHA256" \
  --argjson compressedSize "$COMPRESSED_SIZE" \
  --arg compressedSHA256 "$COMPRESSED_SHA256" \
  --argjson verified "$VERIFY_RAN" \
  --arg generatedAt "$GENERATED_AT" \
  "${growth_arg[@]}" \
  '{
    "$schema_note": "Provenance for internal/diskfmt/ext4golden/golden.img.zst. Regenerate with build/ext4-golden/build.sh; parameter WHYs are in this directory'\''s README.md, not here.",
    e2fsprogs: { repo: "https://github.com/tytso/e2fsprogs.git", tag: $e2fsprogsTag, commit: $e2fsprogsCommit },
    mke2fs: {
      filesystem: "ext4",
      blockSize: 4096,
      goldenSizeMiB: $goldenSizeMB,
      journalSizeMiB: $journalSizeMB,
      features: {
        compat: ["has_journal", "ext_attr", "dir_index"],
        incompat: ["filetype", "meta_bg", "extent", "64bit", "flex_bg", "metadata_csum_seed"],
        roCompat: ["sparse_super", "large_file", "huge_file", "dir_nlink", "extra_isize", "metadata_csum"]
      },
      uuid: $goldenUUID,
      label: "",
      hashSeed: $hashSeed,
      e2fsprogsFakeTime: $fakeTime
    },
    rawImage: { sizeBytes: $rawSize, sha256: $rawSHA256 },
    compressedAsset: { sizeBytes: $compressedSize, sha256: $compressedSHA256, path: "golden.img.zst" },
    verification: {
      ranInContainer: $verified,
      verifiedGrowthCeilingBytes: $verifiedGrowthCeilingBytes,
      note: "See README.md for the full growth-ceiling argument (empirical proof at whatever ceiling this run'\''s build host allowed, plus the documented meta_bg mechanism this generalizes from)."
    },
    generatedAt: $generatedAt
  }' > "${ASSET_DIR}/manifest.json"

echo
echo "Done."
echo "  Raw image:        ${RAW_SIZE} bytes, sha256 ${RAW_SHA256}"
echo "  Compressed asset: ${COMPRESSED_SIZE} bytes, sha256 ${COMPRESSED_SHA256}"
echo "  Written to:       ${ASSET_DIR}/golden.img.zst"
echo "                     ${ASSET_DIR}/manifest.json"
