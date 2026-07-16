# Reproducible cross-build of mainline U-Boot for the Radxa ROCK 4SE.
#
# Everything that determines the output (U-Boot tag, TF-A repo/tag/commit)
# is passed in as build args by build.sh, which reads them from
# ../manifest.json and its own UBOOT_TAG pin -- nothing is pulled in
# unpinned.
#
# Unlike the RK3566/RK3528 boards there is NO rkbin blob fetch here: RK3399's
# DRAM init is open-source in U-Boot's TPL (the defconfig sets CONFIG_TPL=y
# and U-Boot's doc/board/rockchip/rockchip.rst rk3399 recipe exports no
# ROCKCHIP_TPL), and BL31 is compiled from mainline Trusted-Firmware-A in the
# stage below. This is the template for future RK3399-class boards.

FROM debian:bookworm-slim AS build

ARG UBOOT_TAG
ARG TFA_REPO
ARG TFA_TAG
ARG TFA_COMMIT

RUN test -n "$UBOOT_TAG" && test -n "$TFA_REPO" && \
    test -n "$TFA_TAG" && test -n "$TFA_COMMIT"

# build-essential: native host cc/gcc for HOSTCC tools (e.g.
# scripts/basic/fixdep). On an arm64 host, crossbuild-essential-arm64 alone
# happens to pull in a native toolchain too, which masked this dependency
# until CI ran on amd64 runners, where crossbuild-essential-arm64 only
# provides the aarch64-linux-gnu-* CROSS tools.
# gcc-arm-none-eabi: TF-A's rk3399 platform builds the PMU's Cortex-M0
# firmware as a hard dependency of bl31 (it's .incbin'd into pmu_fw.S), and
# that sub-build defaults to the arm-none-eabi- bare-metal toolchain.
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential crossbuild-essential-arm64 gcc-arm-none-eabi \
      git ca-certificates curl \
      make bison flex bc python3 python3-dev python3-setuptools python3-pyelftools \
      swig libssl-dev libgnutls28-dev device-tree-compiler \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# BL31 from source: mainline TF-A at the tag pinned in ../manifest.json,
# with the clone's HEAD verified against the manifest's peeled commit so a
# moved/re-pointed tag fails loudly instead of silently changing the output.
RUN git clone --branch "$TFA_TAG" --depth 1 "$TFA_REPO" /src/trusted-firmware-a \
    && cd /src/trusted-firmware-a \
    && test "$(git rev-parse HEAD)" = "$TFA_COMMIT"

WORKDIR /src/trusted-firmware-a
# LD override: since TF-A's "use clang as a linker" build change (commit
# 6c2e5bf68955, in v2.13+), the default link goes through the compiler
# driver, which mis-places .pmusram with Debian's aarch64-linux-gnu
# toolchains ("region `PMUSRAM' overflowed" — ARM-software/tf-issues#650,
# Debian bug #1118651). Linking directly with GNU ld, which that commit
# explicitly keeps supported via LD=, produces a correctly-placed BL31.
RUN make PLAT=rk3399 \
      CROSS_COMPILE=aarch64-linux-gnu- \
      LD=aarch64-linux-gnu-ld \
      M0_CROSS_COMPILE=arm-none-eabi- \
      -j"$(nproc)" \
      bl31

RUN git clone --branch "$UBOOT_TAG" --depth 1 https://github.com/u-boot/u-boot.git /src/u-boot

COPY bootdelay0.config /src/bootdelay0.config

WORKDIR /src/u-boot
RUN make CROSS_COMPILE=aarch64-linux-gnu- rock-4se-rk3399_defconfig
RUN ./scripts/kconfig/merge_config.sh -m .config /src/bootdelay0.config \
    && make CROSS_COMPILE=aarch64-linux-gnu- olddefconfig

RUN make CROSS_COMPILE=aarch64-linux-gnu- \
      BL31=/src/trusted-firmware-a/build/rk3399/release/bl31/bl31.elf \
      -j"$(nproc)"

# Minimal stage so `docker create`/`docker cp` in build.sh only has to
# handle the two artifacts we care about, not the whole build tree.
FROM scratch AS artifacts
COPY --from=build /src/u-boot/idbloader.img /src/u-boot/u-boot.itb /
