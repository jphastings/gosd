#!/usr/bin/env bash
# Greps a serial log captured by boot-and-grep.sh for
# examples/diskstorage's "gosd disk: filesystem size <N> bytes" line and
# asserts N is close to the 2GiB second virtio disk CI's qemu-disk-ext4 job
# attaches — not internal/diskfmt/ext4golden's fixed 512MiB golden image —
# proving EXT4_IOC_RESIZE_FS actually grew the volume (internal/blockmount's
# runEXT4) rather than the app mounting the golden image as-is.
#
# Usage: scripts/assert-disk-grew.sh <serial-log>
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <serial-log>" >&2
  exit 1
fi
LOG=$1

# Kept in lockstep with the qemu-disk-ext4 job's `truncate -s 2G` disk and
# ext4golden's documented 512MiB image (internal/diskfmt/ext4golden/README.md).
disk_bytes=$((2 * 1024 * 1024 * 1024))
golden_bytes=$((512 * 1024 * 1024))
# The lower bound only needs to clear the golden image by a wide margin —
# ext4/journal/reserved-block overhead on a freshly grown 2GiB volume is a
# few percent at most, so anything past 1GiB is unambiguous proof of growth.
min_bytes=$((golden_bytes * 2))

# -oE (POSIX extended, portable to both GNU and BSD/macOS grep) rather than
# GNU-only -oP/\K, so this also runs from a developer's Mac.
size="$(grep 'gosd disk: filesystem size' "$LOG" | tail -1 | grep -oE '[0-9]+')"
if [ -z "$size" ]; then
  echo "no 'gosd disk: filesystem size <N> bytes' line found in $LOG" >&2
  exit 1
fi

if [ "$size" -le "$min_bytes" ] || [ "$size" -gt "$disk_bytes" ]; then
  echo "filesystem size ${size} bytes is not close to the ${disk_bytes}-byte virtio disk (golden image is only ${golden_bytes} bytes)" >&2
  exit 1
fi

echo "filesystem size ${size} bytes is consistent with the grown ${disk_bytes}-byte disk (golden image is ${golden_bytes} bytes)"
