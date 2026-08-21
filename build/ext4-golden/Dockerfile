# Reproducible build of the ext4 golden image checked into
# internal/diskfmt/ext4golden/: a small pristine ext4 filesystem that gosd's
# disk package raw-copies onto internal drives and grows in place with the
# kernel's EXT4_IOC_RESIZE_FS ioctl (see that directory's README.md for the
# parameter rationale).
#
# e2fsprogs is built from source at the pinned tag/commit below -- Debian's
# packaged mke2fs would drift with every point release, and we need a
# specific, recorded version producing this exact filesystem, the same
# reasoning build/boards/*/uboot's Dockerfiles apply to U-Boot and TF-A.
#
# All inputs that determine the golden image's bytes (e2fsprogs version, the
# mke2fs invocation, the fixed UUID/hash seed/timestamp) are passed in as
# build args by build.sh -- nothing here is pulled in unpinned.

FROM debian:bookworm-slim AS build

ARG E2FSPROGS_TAG
ARG E2FSPROGS_COMMIT

RUN test -n "$E2FSPROGS_TAG" && test -n "$E2FSPROGS_COMMIT"

RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential git ca-certificates pkg-config gettext texinfo zstd \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
RUN git clone --branch "$E2FSPROGS_TAG" --depth 1 https://github.com/tytso/e2fsprogs.git /src/e2fsprogs \
    && cd /src/e2fsprogs \
    && test "$(git rev-parse HEAD)" = "$E2FSPROGS_COMMIT"

WORKDIR /src/e2fsprogs
# --disable-nls: no locale support needed for a build-time tool.
# DESTDIR=/out: install to a throwaway prefix rather than the real /usr, so
# the toolchain packages above stay the only thing apt manages.
RUN ./configure --disable-nls >/dev/null \
    && make -j"$(nproc)" >/dev/null \
    && make install DESTDIR=/out >/dev/null
ENV PATH="/out/sbin:/out/bin:${PATH}"

ARG GOLDEN_UUID
ARG HASH_SEED
ARG FAKE_TIME
ARG GOLDEN_SIZE_MB
ARG JOURNAL_SIZE_MB
ARG FEATURES

RUN test -n "$GOLDEN_UUID" && test -n "$HASH_SEED" \
    && test -n "$FAKE_TIME" && test -n "$GOLDEN_SIZE_MB" \
    && test -n "$JOURNAL_SIZE_MB" && test -n "$FEATURES"

WORKDIR /build
# Parameter rationale lives in ../README.md (internal/diskfmt/ext4golden/) --
# this is the literal invocation those WHYs justify.
#
# -O $FEATURES: the whole feature set, passed in by build.sh so the two
#   goldens differ in exactly one respect and nowhere else. The data golden
#   passes ^resize_inode,meta_bg -- the classic reserved-GDT-blocks
#   online-resize path (resize_inode) tops out around 8TiB with 4KiB
#   blocks/64bit descriptors (its reserved blocks are addressed through the
#   resize inode's single indirect block, capped at blocksize/4 = 1024
#   blocks however large a target -E resize= requests -- verified
#   empirically, see the README), and meta_bg removes that ceiling by
#   storing group descriptors inside each meta block group instead of
#   reserving them contiguously up front, while still supporting online
#   resize (kernel.org's ext4 blockgroup.html: "Existing filesystems can be
#   resized on-line"). The two features are mutually exclusive at format
#   time -- mke2fs refuses combining them. The config golden passes
#   resize_inode instead: a fixed-size partition never approaches that
#   ceiling, and the plainer filesystem is the one any host's e2fsck knows
#   best.
# lazy_itable_init/lazy_journal_init=1: ship without fully zeroing the inode
#   table or journal; the kernel's ext4lazyinit thread finishes after mount
#   (crash-safety argument in the README).
# hash_seed, E2FSPROGS_FAKE_TIME (env below): fixed rather than random/now,
#   for byte-identical rebuilds.
# -J size=$JOURNAL_SIZE_MB: explicit, since the journal does not grow with
#   the filesystem -- see the README for why this size was chosen.
# -U/-L: fixed placeholder UUID, empty label; a later bean (gosd-apmv) stamps
#   real per-volume values into the superblock at format time.
RUN dd if=/dev/zero of=/build/golden.img bs=1M count="$GOLDEN_SIZE_MB" \
    && E2FSPROGS_FAKE_TIME="$FAKE_TIME" mke2fs -q -F -t ext4 -b 4096 \
         -O "$FEATURES" \
         -E lazy_itable_init=1,lazy_journal_init=1,hash_seed="$HASH_SEED",root_owner=0:0 \
         -J size="$JOURNAL_SIZE_MB" \
         -U "$GOLDEN_UUID" \
         -L "" \
         -I 256 \
         /build/golden.img \
    && dumpe2fs -h /build/golden.img > /build/golden.dumpe2fs.txt 2>&1

COPY verify.sh /build/verify.sh
RUN chmod +x /build/verify.sh

# Minimal stage so build.sh's `docker create`/`docker cp` only has to handle
# the golden image, not the whole e2fsprogs build tree -- mirrors
# build/boards/rock-4se/uboot/Dockerfile's artifacts stage.
FROM scratch AS artifacts
COPY --from=build /build/golden.img /golden.img
COPY --from=build /build/golden.dumpe2fs.txt /golden.dumpe2fs.txt
