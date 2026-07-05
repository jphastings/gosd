#!/usr/bin/env bash
# Packages CI-built board artifacts into the tarballs + manifest.json that
# .github/workflows/build-artifacts.yml publishes as a GitHub Release (bean
# gosd-wtpa). Kept as a standalone script (rather than inline workflow YAML)
# so it can be exercised locally without Docker, real kernel builds, or
# network access — see "Testing this script" in docs/artifacts.md.
#
# Usage:
#   package.sh <version> <staging-dir> <output-dir>
#
# staging-dir must contain one subdirectory per board, named exactly like
# the board's --board ID (e.g. "pi-zero-2w", "radxa-zero-3e"). Every regular
# file directly inside a board's subdirectory is packaged into
# output-dir/<board>.tar.zst and listed in output-dir/manifest.json, keyed by
# its base name (matching the ArtifactRef.Name each board profile expects —
# see internal/boards/pizero2w and internal/boards/radxazero3e).
#
# One optional file per board subdirectory is treated specially:
# "source.json", if present, is not packaged into the tarball; its content
# is copied verbatim into manifest.json as boards.<board>.source, recording
# the upstream repo/commit/config path each compiled artifact came from (for
# GPL provenance). Its shape is up to the caller; build-artifacts.yml
# generates one per board from the pinned values in each build.sh.
#
# Output: output-dir/<board>.tar.zst for every board subdirectory found, and
# output-dir/manifest.json: {version, boards: {<board>: {source, files:
# [{name, sha256, size}]}}} — the schema internal/artifacts.Manifest parses.
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is required to build manifest.json; install jq and try again" >&2
  exit 1
fi
if [ "$#" -ne 3 ]; then
  echo "usage: $0 <version> <staging-dir> <output-dir>" >&2
  exit 1
fi

VERSION="$1"
STAGING_DIR="$2"
OUT_DIR="$3"

if [ ! -d "$STAGING_DIR" ]; then
  echo "error: staging dir $STAGING_DIR does not exist" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

manifest=$(jq -n --arg version "$VERSION" '{version: $version, boards: {}}')

shopt -s nullglob
board_dirs=("$STAGING_DIR"/*/)
if [ "${#board_dirs[@]}" -eq 0 ]; then
  echo "error: $STAGING_DIR has no board subdirectories" >&2
  exit 1
fi

for board_path in "${board_dirs[@]}"; do
  board="$(basename "$board_path")"
  echo "==> Packaging $board"

  source_json="${board_path}source.json"
  if [ -f "$source_json" ]; then
    source_obj="$(cat "$source_json")"
  else
    source_obj='{}'
  fi

  files=()
  while IFS= read -r -d '' f; do
    name="$(basename "$f")"
    [ "$name" = "source.json" ] && continue
    files+=("$name")
  done < <(find "$board_path" -maxdepth 1 -type f -print0)

  if [ "${#files[@]}" -eq 0 ]; then
    echo "error: $board_path has no artifact files to package (besides source.json)" >&2
    exit 1
  fi

  tarball="$OUT_DIR/$board.tar.zst"
  tar --zstd -cf "$tarball" -C "$board_path" --exclude=source.json "${files[@]}"
  echo "    wrote $tarball ($(du -h "$tarball" | cut -f1))"

  board_files='[]'
  for name in "${files[@]}"; do
    path="${board_path}${name}"
    sha256="$(sha256sum "$path" | cut -d' ' -f1)"
    size="$(stat -c%s "$path" 2>/dev/null || stat -f%z "$path")"
    entry="$(jq -n --arg name "$name" --arg sha256 "$sha256" --argjson size "$size" \
      '{name: $name, sha256: $sha256, size: $size}')"
    board_files="$(jq -c --argjson entry "$entry" '. + [$entry]' <<<"$board_files")"
  done

  board_entry="$(jq -n --argjson source "$source_obj" --argjson files "$board_files" '{source: $source, files: $files}')"
  manifest="$(jq --arg board "$board" --argjson entry "$board_entry" '.boards[$board] = $entry' <<<"$manifest")"
done

echo "$manifest" | jq '.' >"$OUT_DIR/manifest.json"
echo "==> Wrote $OUT_DIR/manifest.json"
