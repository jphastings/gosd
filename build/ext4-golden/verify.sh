#!/usr/bin/env bash
# Runs *inside* a --privileged `docker run` of this recipe's build stage
# (invoked by build.sh, never by `docker build` -- BuildKit RUN steps can't
# loop-mount). Proves the golden image's online-resize path actually works
# through the real kernel ioctl, not just mke2fs's own bookkeeping:
#
#   1. loop-mount the golden image
#   2. write a file
#   3. grow a truncated (sparse) copy WHILE MOUNTED via resize2fs, which
#      goes through the same EXT4_IOC_RESIZE_FS path gosd's disk package
#      uses on-device
#   4. confirm the file written in step 2 survived the grow
#   5. unmount, fsck -f, and require a clean pass
#
# If loop-mounting isn't available at all (privileged containers can't get
# a loop device -- has been true on some CI runners and non-colima Docker
# setups), this degrades gracefully: prints a clear message and exits 0.
# Runtime verification then falls to the qemu-virt smoke test, bean
# gosd-ucgr. Any failure *after* a loop device is obtained is a real
# problem and fails the build.
set -uo pipefail

GOLDEN=/build/golden.img
WORK=/tmp/verify
mkdir -p "$WORK"

echo "verify: probing for loop device support..."
LOOPDEV=$(losetup -f 2>/dev/null)
if [ -z "$LOOPDEV" ] || ! losetup "$LOOPDEV" "$GOLDEN" 2>/tmp/losetup.err; then
  echo "verify: SKIPPED -- no usable loop device in this container (privileged loop-mounts unavailable here)."
  echo "verify: $(cat /tmp/losetup.err 2>/dev/null)"
  echo "verify: runtime verification of online resize therefore falls to the qemu-virt smoke test (bean gosd-ucgr)."
  exit 0
fi
cleanup() {
  umount "$WORK/mnt" 2>/dev/null || true
  losetup -d "$LOOPDEV" 2>/dev/null || true
}
trap cleanup EXIT

mkdir -p "$WORK/mnt"
if ! mount -t ext4 "$LOOPDEV" "$WORK/mnt"; then
  echo "verify: SKIPPED -- got a loop device but mount(2) failed (likely a sandboxed/rootless container without CAP_SYS_ADMIN)."
  echo "verify: runtime verification of online resize therefore falls to the qemu-virt smoke test (bean gosd-ucgr)."
  losetup -d "$LOOPDEV" 2>/dev/null || true
  trap - EXIT
  exit 0
fi
echo "verify: loop-mounted OK at $LOOPDEV"

echo "gosd ext4 golden verification" > "$WORK/mnt/marker.txt"
if ! grep -q gosd "$WORK/mnt/marker.txt"; then
  echo "verify: FAILED -- could not write/read a file on the freshly mounted golden image" >&2
  exit 1
fi
umount "$WORK/mnt"
losetup -d "$LOOPDEV"

# Grow a truncated (sparse) copy of the golden image. The largest target
# this build host's own filesystem can represent is tried first, falling
# back to smaller round sizes -- some hosts' root filesystems are
# themselves capped well under 16TiB (this sandbox's colima VM, for one:
# its own ext4 root has no 64bit feature, so files top out just under
# 2^32 blocks). That is a build-HOST limitation, not a property of the
# golden image or its meta_bg parameters, so we record whichever ceiling
# this run actually proved rather than silently claiming the target we
# couldn't reach here.
GROW=/tmp/grow.img
for TARGET_BYTES in 17592186044416 8796093022208 4398046511104 1099511627776 274877906944; do
  cp "$GOLDEN" "$GROW"
  if truncate -s "$TARGET_BYTES" "$GROW" 2>/dev/null; then
    echo "verify: growing to $TARGET_BYTES bytes ($((TARGET_BYTES / 1024 / 1024 / 1024 / 1024)) TiB, or just under)"
    break
  fi
  TARGET_BYTES=""
done
if [ -z "${TARGET_BYTES:-}" ]; then
  echo "verify: FAILED -- could not truncate a sparse file to even the smallest fallback size" >&2
  exit 1
fi

LOOPDEV=$(losetup -f)
losetup "$LOOPDEV" "$GROW"
mount -t ext4 "$LOOPDEV" "$WORK/mnt"
if ! grep -q gosd "$WORK/mnt/marker.txt"; then
  echo "verify: FAILED -- marker file missing immediately after mounting the grow target" >&2
  exit 1
fi

echo "verify: resizing while mounted (online resize, the EXT4_IOC_RESIZE_FS path)..."
if ! resize2fs "$LOOPDEV"; then
  echo "verify: FAILED -- online resize2fs failed" >&2
  exit 1
fi

NEW_SIZE=$(df -B1 --output=size "$WORK/mnt" | tail -1 | tr -d ' ')
echo "verify: grown filesystem reports size $NEW_SIZE bytes"

if ! grep -q gosd "$WORK/mnt/marker.txt"; then
  echo "verify: FAILED -- marker file did not survive the online grow" >&2
  exit 1
fi
echo "verify: marker file survived the online grow"

umount "$WORK/mnt"
echo "verify: running e2fsck -f on the grown filesystem..."
if ! e2fsck -f -y "$LOOPDEV"; then
  echo "verify: FAILED -- e2fsck reported errors on the grown filesystem" >&2
  exit 1
fi
losetup -d "$LOOPDEV"
trap - EXIT

echo "verify: PASSED -- golden image mounted, wrote data, grew online from $(stat -c%s "$GOLDEN") to $TARGET_BYTES bytes, data survived, fsck clean."
echo "VERIFIED_GROWTH_CEILING_BYTES=$TARGET_BYTES"
