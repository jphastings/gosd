#!/usr/bin/env bash
# Regenerates the ext4 golden images checked into
# internal/diskfmt/ext4golden/: builds e2fsprogs from source inside Docker,
# runs mke2fs with GoSD's pinned parameters, verifies the mount/resize path
# in a privileged container, then compresses and stamps the checked-in
# assets + their manifests.
#
# Usage: ./build.sh [data|config|all]     (default: all)
#
# There are two goldens, because one image cannot serve both jobs:
#
#   data    512MiB seed for /data and for the emmc/disk packages. It grows to
#           the volume's real size on first boot, so its 128MiB journal is
#           sized for the GROWN filesystem's whole life (a journal can never
#           be resized after format) and it ships ^resize_inode,meta_bg to
#           escape resize_inode's ~8TiB growth ceiling.
#   config  32MiB seed for the config partition, which is fixed-size and in
#           the common case never grows. Journal at the ext4 minimum (1024
#           blocks = 4MiB), and a plain, standard feature set -- resize_inode
#           rather than meta_bg -- because the more ordinary the filesystem,
#           the better any host's e2fsck can repair it.
#
# Pinned inputs (edit here to change a golden image):
#   - e2fsprogs release tag + commit: E2FSPROGS_TAG / E2FSPROGS_COMMIT below.
#   - mke2fs parameters (feature set, block/journal size, fixed UUID/hash
#     seed): the per-variant block in variantParams below, plus the -E flags
#     baked into Dockerfile's mke2fs RUN step.
#     WHY each one was chosen is recorded in ../../internal/diskfmt/ext4golden/README.md,
#     not repeated here.
set -euo pipefail

E2FSPROGS_TAG="v1.47.4"
E2FSPROGS_COMMIT="7ee1d505ef3b37831215f490411f346fe57e9053"

# A fixed epoch (2025-01-01T00:00:00Z), not "now" -- every regen produces
# the same on-disk timestamps regardless of when build.sh actually runs,
# which is what makes the raw image byte-reproducible.
FAKE_TIME="1735689600"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${SCRIPT_DIR}/out"
ASSET_DIR="$(cd "${SCRIPT_DIR}/../../internal/diskfmt/ext4golden" && pwd)"

# variantParams sets every per-golden parameter for $1. The fixed UUIDs and
# hash seeds are arbitrary placeholders -- not real per-volume identity.
# gosd-apmv stamps a fresh random UUID (and the real label) into the
# superblock at format time; these values only need to be fixed, not secret
# or unique. They differ between the two goldens only so that a raw image
# can be told apart by eye.
variantParams() {
  case "$1" in
    data)
      GOLDEN_SIZE_MB="512"
      JOURNAL_SIZE_MB="128"
      GOLDEN_UUID="4c1a41c8-20b8-4c50-8399-7fae324e8398"
      HASH_SEED="da89e13f-1cf4-4015-a4e0-0e9abbd2aabd"
      # ^resize_inode,meta_bg: see the README's "Why meta_bg, not
      # resize_inode" -- a growth-ceiling argument that only applies to a
      # golden which grows.
      FEATURES="^resize_inode,meta_bg,sparse_super,large_file,filetype,dir_index,ext_attr,extent,huge_file,flex_bg,metadata_csum,metadata_csum_seed,64bit,dir_nlink,extra_isize"
      FEATURES_COMPAT='["has_journal", "ext_attr", "dir_index"]'
      FEATURES_INCOMPAT='["filetype", "meta_bg", "extent", "64bit", "flex_bg", "metadata_csum_seed"]'
      FEATURES_ROCOMPAT='["sparse_super", "large_file", "huge_file", "dir_nlink", "extra_isize", "metadata_csum"]'
      ASSET_IMAGE="golden.img.zst"
      ASSET_MANIFEST="manifest.json"
      VERIFY_NOTE="See README.md for the full growth-ceiling argument (empirical proof at whatever ceiling this run's build host allowed, plus the documented meta_bg mechanism this generalizes from)."
      # Largest first; verify.sh falls back until the build host's own
      # filesystem can represent a file that big.
      GROW_TARGETS="17592186044416 8796093022208 4398046511104 1099511627776 274877906944"
      ;;
    config)
      GOLDEN_SIZE_MB="32"
      JOURNAL_SIZE_MB="4"
      GOLDEN_UUID="d33ae914-c738-4bea-ba4d-99fe3c1bf25d"
      HASH_SEED="a8bade99-e2c6-4e0d-a634-1e4f2a5d764c"
      # resize_inode, NOT meta_bg: a fixed-size partition cannot reach the
      # ceiling meta_bg exists to remove, and resize_inode is what an
      # ordinary mkfs.ext4 produces. Everything else matches the data
      # golden feature for feature.
      FEATURES="resize_inode,sparse_super,large_file,filetype,dir_index,ext_attr,extent,huge_file,flex_bg,metadata_csum,metadata_csum_seed,64bit,dir_nlink,extra_isize"
      FEATURES_COMPAT='["has_journal", "ext_attr", "resize_inode", "dir_index"]'
      FEATURES_INCOMPAT='["filetype", "extent", "64bit", "flex_bg", "metadata_csum_seed"]'
      FEATURES_ROCOMPAT='["sparse_super", "large_file", "huge_file", "dir_nlink", "extra_isize", "metadata_csum"]'
      ASSET_IMAGE="config-golden.img.zst"
      ASSET_MANIFEST="config-manifest.json"
      VERIFY_NOTE="Proves the fixed-size path (mount, write, fsck) and that an oversized --config-size still grows through resize_inode's reserved GDT blocks. See README.md."
      # 1GiB is an implausibly large --config-size and sits comfortably
      # inside resize_inode's reserved-GDT headroom; there is nothing to
      # learn from attempting terabytes against a golden not meant to reach
      # them.
      GROW_TARGETS="1073741824"
      ;;
    *)
      echo "error: unknown golden variant '$1'; expected 'data' or 'config'" >&2
      exit 2
      ;;
  esac
}

buildVariant() {
  local variant="$1"
  variantParams "$variant"

  local build_tag="gosd-ext4-golden-build-${variant}"
  local artifacts_tag="gosd-ext4-golden-artifacts-${variant}"
  local variant_out="${OUT_DIR}/${variant}"

  echo
  echo "=== ${variant} golden: ${GOLDEN_SIZE_MB}MiB, ${JOURNAL_SIZE_MB}MiB journal ==="
  echo "Building e2fsprogs ${E2FSPROGS_TAG} (@ ${E2FSPROGS_COMMIT:0:12}) and the ext4 golden image..."

  local build_args=(
    --build-arg "E2FSPROGS_TAG=${E2FSPROGS_TAG}"
    --build-arg "E2FSPROGS_COMMIT=${E2FSPROGS_COMMIT}"
    --build-arg "GOLDEN_UUID=${GOLDEN_UUID}"
    --build-arg "HASH_SEED=${HASH_SEED}"
    --build-arg "FAKE_TIME=${FAKE_TIME}"
    --build-arg "GOLDEN_SIZE_MB=${GOLDEN_SIZE_MB}"
    --build-arg "JOURNAL_SIZE_MB=${JOURNAL_SIZE_MB}"
    --build-arg "FEATURES=${FEATURES}"
  )

  docker build --target build --tag "$build_tag" "${build_args[@]}" "$SCRIPT_DIR"
  docker build --target artifacts --tag "$artifacts_tag" "${build_args[@]}" "$SCRIPT_DIR"

  mkdir -p "$variant_out"
  local container_id
  container_id=$(docker create "$artifacts_tag" placeholder)
  trap 'docker rm -f "$container_id" >/dev/null 2>&1 || true' EXIT
  docker cp "${container_id}:/golden.img" "${variant_out}/golden.img"
  docker cp "${container_id}:/golden.dumpe2fs.txt" "${variant_out}/golden.dumpe2fs.txt"
  docker rm -f "$container_id" >/dev/null 2>&1 || true
  trap - EXIT

  echo
  echo "Running in-container verification (privileged loop-mount + resize2fs + fsck)..."
  local verify_log="${variant_out}/verify.log"
  local verify_ran=false
  local growth_ceiling_bytes=""
  if docker run --rm --privileged -e "GROW_TARGETS=${GROW_TARGETS}" "$build_tag" /build/verify.sh 2>&1 | tee "$verify_log"; then
    if grep -q "verify: PASSED" "$verify_log"; then
      verify_ran=true
      growth_ceiling_bytes=$(grep -oE 'VERIFIED_GROWTH_CEILING_BYTES=[0-9]+' "$verify_log" | cut -d= -f2)
      echo "Verification PASSED. Growth ceiling proven this run: ${growth_ceiling_bytes} bytes."
    else
      echo "Verification SKIPPED (see ${verify_log} for why) -- runtime verification falls to the qemu-virt smoke test (bean gosd-ucgr)."
    fi
  else
    echo "error: in-container verification FAILED -- see ${verify_log}" >&2
    exit 1
  fi

  echo
  echo "Compressing with zstd (inside the pinned build container)..."
  docker run --rm -v "${variant_out}:/hostout" "$build_tag" \
    zstd -19 -q -f -o /hostout/golden.img.zst /build/golden.img

  local raw_sha256 compressed_sha256 compressed_size raw_size
  raw_sha256=$(shasum -a 256 "${variant_out}/golden.img" | cut -d' ' -f1)
  compressed_sha256=$(shasum -a 256 "${variant_out}/golden.img.zst" | cut -d' ' -f1)
  compressed_size=$(stat -f%z "${variant_out}/golden.img.zst" 2>/dev/null || stat -c%s "${variant_out}/golden.img.zst")
  raw_size=$(stat -f%z "${variant_out}/golden.img" 2>/dev/null || stat -c%s "${variant_out}/golden.img")

  cp "${variant_out}/golden.img.zst" "${ASSET_DIR}/${ASSET_IMAGE}"

  local generated_at
  local growth_arg
  generated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  if [ -n "$growth_ceiling_bytes" ]; then
    growth_arg=(--argjson verifiedGrowthCeilingBytes "$growth_ceiling_bytes")
  else
    growth_arg=(--argjson verifiedGrowthCeilingBytes null)
  fi

  jq -n \
    --arg variant "$variant" \
    --arg e2fsprogsTag "$E2FSPROGS_TAG" \
    --arg e2fsprogsCommit "$E2FSPROGS_COMMIT" \
    --arg goldenUUID "$GOLDEN_UUID" \
    --arg hashSeed "$HASH_SEED" \
    --arg fakeTime "$FAKE_TIME" \
    --arg assetPath "$ASSET_IMAGE" \
    --arg verifyNote "$VERIFY_NOTE" \
    --argjson featuresCompat "$FEATURES_COMPAT" \
    --argjson featuresIncompat "$FEATURES_INCOMPAT" \
    --argjson featuresRoCompat "$FEATURES_ROCOMPAT" \
    --argjson goldenSizeMB "$GOLDEN_SIZE_MB" \
    --argjson journalSizeMB "$JOURNAL_SIZE_MB" \
    --argjson rawSize "$raw_size" \
    --arg rawSHA256 "$raw_sha256" \
    --argjson compressedSize "$compressed_size" \
    --arg compressedSHA256 "$compressed_sha256" \
    --argjson verified "$verify_ran" \
    --arg generatedAt "$generated_at" \
    "${growth_arg[@]}" \
    '{
      "$schema_note": ("Provenance for internal/diskfmt/ext4golden/" + $assetPath + ". Regenerate with build/ext4-golden/build.sh " + $variant + "; parameter WHYs are in that directory'\''s README.md, not here."),
      variant: $variant,
      e2fsprogs: { repo: "https://github.com/tytso/e2fsprogs.git", tag: $e2fsprogsTag, commit: $e2fsprogsCommit },
      mke2fs: {
        filesystem: "ext4",
        blockSize: 4096,
        goldenSizeMiB: $goldenSizeMB,
        journalSizeMiB: $journalSizeMB,
        features: {
          compat: $featuresCompat,
          incompat: $featuresIncompat,
          roCompat: $featuresRoCompat
        },
        uuid: $goldenUUID,
        label: "",
        hashSeed: $hashSeed,
        e2fsprogsFakeTime: $fakeTime
      },
      rawImage: { sizeBytes: $rawSize, sha256: $rawSHA256 },
      compressedAsset: { sizeBytes: $compressedSize, sha256: $compressedSHA256, path: $assetPath },
      verification: {
        ranInContainer: $verified,
        verifiedGrowthCeilingBytes: $verifiedGrowthCeilingBytes,
        note: $verifyNote
      },
      generatedAt: $generatedAt
    }' > "${ASSET_DIR}/${ASSET_MANIFEST}"

  echo
  echo "Done (${variant})."
  echo "  Raw image:        ${raw_size} bytes, sha256 ${raw_sha256}"
  echo "  Compressed asset: ${compressed_size} bytes, sha256 ${compressed_sha256}"
  echo "  Written to:       ${ASSET_DIR}/${ASSET_IMAGE}"
  echo "                     ${ASSET_DIR}/${ASSET_MANIFEST}"
}

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker is required to build the ext4 golden images; install Docker and try again" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is required to write the manifests; install jq (e.g. 'brew install jq') and try again" >&2
  exit 1
fi

case "${1:-all}" in
  all)
    buildVariant data
    buildVariant config
    ;;
  data) buildVariant data ;;
  config) buildVariant config ;;
  *)
    echo "usage: $0 [data|config|all]" >&2
    exit 2
    ;;
esac
