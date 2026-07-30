#!/usr/bin/env bash
# Boots a qemu-virt image once, waits until the app answers HTTP (i.e. the
# whole boot sequence, including any first-boot data-partition work,
# completed), shuts qemu down, and asserts each expected string appears in
# the captured serial log.
#
# Usage: scripts/boot-and-grep.sh <path-to-image.img> <serial-log> <expected-string>...
#
# BOOT_WAIT_FOR=<string> waits for that string on the serial console instead
# of an HTTP response — for boots that deliberately never reach the app,
# like the corrupt-data-partition halt.
#
# Written for CI's qemu-expand-data job: booting the same image twice with
# different expected strings proves first-boot work happened exactly once
# and survived the (abrupt, power-cut-like) end of the previous boot.
set -euo pipefail

if [ $# -lt 3 ]; then
  echo "usage: $0 <path-to-image.img> <serial-log> <expected-string>..." >&2
  exit 1
fi
IMG=$1
LOG=$2
shift 2

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"${SCRIPT_DIR}/qemu-run.sh" "$IMG" > "$LOG" 2>&1 &
runner_pid=$!

# amd64 CI runners TCG-emulate arm64: the timeout is generous headroom,
# not an expectation (an arm64 host boots in seconds).
booted=0
for _ in $(seq 1 240); do
  if [ -n "${BOOT_WAIT_FOR:-}" ]; then
    if grep -qF -- "$BOOT_WAIT_FOR" "$LOG" 2>/dev/null; then
      sleep 2 # let the lines after the awaited one land in the log too
      booted=1
      break
    fi
  elif [ -n "$(curl -s -m 2 http://localhost:8080/ || true)" ]; then
    booted=1
    break
  fi
  if ! kill -0 "$runner_pid" 2>/dev/null; then
    echo "qemu exited before the boot got anywhere" >&2
    break
  fi
  sleep 1
done

kill "$runner_pid" 2>/dev/null || true
pkill -f qemu-system-aarch64 2>/dev/null || true
wait "$runner_pid" 2>/dev/null || true

fail() {
  echo "::group::Captured serial log"
  cat "$LOG"
  echo "::endgroup::"
  echo "$1" >&2
  exit 1
}

if [ "$booted" -ne 1 ]; then
  fail "the boot never reached ${BOOT_WAIT_FOR:-an HTTP response on http://localhost:8080} within 240s"
fi
for want in "$@"; do
  if ! grep -qF -- "$want" "$LOG"; then
    fail "serial log is missing: $want"
  fi
done
echo "boot OK; serial log contains all expected lines"
