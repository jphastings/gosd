# Reproducible cross-build of mainline U-Boot for the Radxa Cubie A5E.
#
# Everything that determines the output (U-Boot tag, TF-A repo/branch/commit)
# is passed in as build args by build.sh, which reads them from
# ../manifest.json and its own UBOOT_TAG pin -- nothing is pulled in
# unpinned.
#
# Like rock-4se there is NO rkbin-style blob fetch here: the A527's DRAM init
# is open-source in mainline U-Boot's SPL (CONFIG_MACH_SUN55I_A523 selects
# arch/arm/mach-sunxi/dram_sun55i_a523.c), and BL31 is compiled from source in
# the stage below. Unlike rock-4se's TF-A pin, this one is a moving BRANCH on
# a fork (jernejsk/arm-trusted-firmware, branch a523) rather than a tagged
# release of the upstream project -- mainline Trusted-Firmware-A has no
# sun55i_a523 platform yet (bean gosd-jpc8). So this Dockerfile fetches the
# pinned COMMIT directly instead of cloning the branch and checking its HEAD:
# a moved branch tip must not break this build, since the pinned commit stays
# fetchable from the fork regardless of where the branch points today.

FROM debian:bookworm-slim AS build

ARG UBOOT_TAG
ARG TFA_REPO
ARG TFA_COMMIT

RUN test -n "$UBOOT_TAG" && test -n "$TFA_REPO" && test -n "$TFA_COMMIT"

# build-essential: native host cc/gcc for HOSTCC tools (e.g.
# scripts/basic/fixdep). On an arm64 host, crossbuild-essential-arm64 alone
# happens to pull in a native toolchain too, which masked this dependency
# until CI ran on amd64 runners, where crossbuild-essential-arm64 only
# provides the aarch64-linux-gnu-* CROSS tools.
#
# No gcc-arm-none-eabi here: that was rk3399's TF-A quirk (its platform
# builds a Cortex-M0 PMU firmware as a hard dependency of bl31). The A523 has
# no such sub-build.
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential crossbuild-essential-arm64 \
      git ca-certificates curl \
      make bison flex bc python3 python3-dev python3-setuptools python3-pyelftools \
      swig libssl-dev libgnutls28-dev device-tree-compiler \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# BL31 from source: jernejsk/arm-trusted-firmware's a523 branch, at the
# commit pinned in ../manifest.json. Fetched by commit rather than cloned by
# branch name and verified after the fact -- the branch can move at any time
# (it's someone else's development branch, not a release tag), and a moved
# branch must not break this build so long as the pinned commit is still
# reachable from the fork.
RUN git init /src/trusted-firmware-a \
    && cd /src/trusted-firmware-a \
    && git remote add origin "$TFA_REPO" \
    && git fetch --depth 1 origin "$TFA_COMMIT" \
    && git checkout FETCH_HEAD

WORKDIR /src/trusted-firmware-a
RUN make PLAT=sun55i_a523 \
      CROSS_COMPILE=aarch64-linux-gnu- \
      -j"$(nproc)" \
      bl31

RUN git clone --branch "$UBOOT_TAG" --depth 1 https://github.com/u-boot/u-boot.git /src/u-boot

COPY bootdelay0.config /src/bootdelay0.config

WORKDIR /src/u-boot
RUN make CROSS_COMPILE=aarch64-linux-gnu- radxa-cubie-a5e_defconfig
RUN ./scripts/kconfig/merge_config.sh -m .config /src/bootdelay0.config \
    && make CROSS_COMPILE=aarch64-linux-gnu- olddefconfig

# BL31= takes the raw bl31.bin (not the .elf rock-4se uses) per U-Boot's
# sunxi build docs. SCP=/dev/null: the A523 uses no SCP firmware, and without
# this U-Boot's build prints a "SCP file not found" note and still succeeds,
# but pointing it at /dev/null makes the absence explicit and silences it.
RUN make CROSS_COMPILE=aarch64-linux-gnu- \
      BL31=/src/trusted-firmware-a/build/sun55i_a523/release/bl31.bin \
      SCP=/dev/null \
      -j"$(nproc)"

# Minimal stage so `docker create`/`docker cp` in build.sh only has to
# handle the one artifact we care about, not the whole build tree.
FROM scratch AS artifacts
COPY --from=build /src/u-boot/u-boot-sunxi-with-spl.bin /
