# Reproducible cross-build of mainline U-Boot for the Radxa Zero 3E/3W.
#
# Everything that determines the output (U-Boot tag, rkbin commit, blob
# hashes) is passed in as build args by build.sh, which reads them from
# ../manifest.json and this Dockerfile's own UBOOT_TAG default -- nothing is
# pulled in unpinned.

FROM debian:bookworm-slim AS build

ARG UBOOT_TAG
ARG RKBIN_COMMIT
ARG RKBIN_DDR_PATH
ARG RKBIN_DDR_SHA256
ARG RKBIN_BL31_PATH
ARG RKBIN_BL31_SHA256

RUN test -n "$UBOOT_TAG" && test -n "$RKBIN_COMMIT" && \
    test -n "$RKBIN_DDR_PATH" && test -n "$RKBIN_DDR_SHA256" && \
    test -n "$RKBIN_BL31_PATH" && test -n "$RKBIN_BL31_SHA256"

# build-essential: native host cc/gcc for HOSTCC tools (e.g.
# scripts/basic/fixdep). On an arm64 host, crossbuild-essential-arm64 alone
# happens to pull in a native toolchain too, which masked this dependency
# until CI ran on amd64 runners, where crossbuild-essential-arm64 only
# provides the aarch64-linux-gnu-* CROSS tools.
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential crossbuild-essential-arm64 \
      git ca-certificates curl \
      make bison flex bc python3 python3-dev python3-setuptools python3-pyelftools \
      swig libssl-dev libgnutls28-dev device-tree-compiler \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# rkbin blobs: fetched by pinned commit + path, verified against the sha256
# pinned in manifest.json before use. This is the one place upstream binary
# blobs enter the build (see repo root CLAUDE.md's no-rehosting policy).
RUN curl -fsSL "https://raw.githubusercontent.com/rockchip-linux/rkbin/${RKBIN_COMMIT}/${RKBIN_DDR_PATH}" -o /src/rk3568_ddr_tpl.bin \
    && echo "${RKBIN_DDR_SHA256}  /src/rk3568_ddr_tpl.bin" | sha256sum -c -
RUN curl -fsSL "https://raw.githubusercontent.com/rockchip-linux/rkbin/${RKBIN_COMMIT}/${RKBIN_BL31_PATH}" -o /src/rk3568_bl31.elf \
    && echo "${RKBIN_BL31_SHA256}  /src/rk3568_bl31.elf" | sha256sum -c -

RUN git clone --branch "$UBOOT_TAG" --depth 1 https://github.com/u-boot/u-boot.git /src/u-boot

COPY bootdelay0.config /src/bootdelay0.config

WORKDIR /src/u-boot
RUN make CROSS_COMPILE=aarch64-linux-gnu- radxa-zero-3-rk3566_defconfig
RUN ./scripts/kconfig/merge_config.sh -m .config /src/bootdelay0.config \
    && make CROSS_COMPILE=aarch64-linux-gnu- olddefconfig

RUN make CROSS_COMPILE=aarch64-linux-gnu- \
      BL31=/src/rk3568_bl31.elf \
      ROCKCHIP_TPL=/src/rk3568_ddr_tpl.bin \
      -j"$(nproc)"

# Minimal stage so `docker create`/`docker cp` in build.sh only has to
# handle the two artifacts we care about, not the whole build tree.
FROM scratch AS artifacts
COPY --from=build /src/u-boot/idbloader.img /src/u-boot/u-boot.itb /
