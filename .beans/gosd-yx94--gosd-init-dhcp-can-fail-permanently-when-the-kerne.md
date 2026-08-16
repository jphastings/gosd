---
# gosd-yx94
title: 'gosd-init: DHCP can fail permanently when the kernel CRNG is unseeded (cubie-a5e has no entropy source)'
status: todo
type: bug
created_at: 2026-08-16T19:25:39Z
updated_at: 2026-08-16T19:25:39Z
---

Found on the Cubie A5E bench (bean gosd-6pfn). One boot in a handful never
joined the network at all; the rest were fine.

## Observed

Failing boot — the only DHCP line in the whole log:

```
[gosd] DHCP discovery on eth0 failed: DHCP discover/request on eth0: unable to
receive an offer: unable to create a discovery request: could not get random
number: context deadline exceeded; retrying in 323.565781ms
```

It said "retrying" but nothing further was logged, and the board had no
address for the remaining ~3.5 minutes it was observed: `ping`/`curl` to its
mDNS name and to the address U-Boot had leased both failed, and no mDNS
responder ever started. A power cycle cleared it.

A later boot, instrumented with a throwaway app sampling kernel state:

```
t=1.34    carrier=0  operstate=down  entropy=1    cryptorand=ok
t=23.28   carrier=1  operstate=up    entropy=256  cryptorand=ok
```

— i.e. **`entropy_avail` is 1 at the moment gosd-init starts networking**. That
boot got a lease, answered mDNS (`ping benchdiag.local` → 0.4ms) and served
HTTP, so this is a startup race, not a broken NIC.

## Why this board loses the race

The Cubie A5E has **no entropy source available to Linux at all** at our pins:

- No Allwinner RNG/TRNG driver is enabled in the cubie-a5e kernel. The many
  `CONFIG_HW_RANDOM_*=y` entries it does carry are defconfig leakage for other
  vendors' SoCs (Broadcom, Meson, Exynos…) — none exist on an A527.
- The board DTB at our kernel pin has **no crypto/rng node at all**, and
  mainline's `sun8i-ce` crypto driver (which provides `..._TRNG`) has no
  sun55i/A523 compatible at v6.18 — so enabling the symbol would bind nothing.
- `CONFIG_RANDOM_TRUST_BOOTLOADER` is not set, so no bootloader seed is
  credited either.

Contrast rock-4se, which ships `CONFIG_HW_RANDOM_ROCKCHIP=y` — a real RNG for
its SoC. So the CRNG here is seeded only by interrupt timing, which took on the
order of 20 seconds in the instrumented boot. gosd-init asks for randomness
well before that.

## Why it matters beyond DHCP

Anything needing cryptographic randomness early is exposed on this board: TLS
from the app, and both ingress shims. DHCP is simply the first caller, and the
one that decides whether the device is reachable at all.

## Fix directions (not implemented)

1. **Make gosd-init's DHCP not depend on a seeded CRNG.** A DHCP transaction ID
   needs uniqueness, not cryptographic quality; it should never be the reason a
   board is unreachable. Worth checking what the DHCP library requires and
   whether it can be handed a non-blocking source.
2. **Make the retry loop actually recover and say so.** It logged one failure,
   claimed a 323ms retry, and then went quiet for minutes — whatever happened
   next was invisible. A boot that never gets an address must keep saying so.
3. **Give the kernel a seed**: check whether the pinned TF-A fork implements the
   SMCCC TRNG (`CONFIG_HW_RANDOM_ARM_SMCCC_TRNG` is already =y, so this may be
   free), and/or `CONFIG_RANDOM_TRUST_BOOTLOADER` plus a U-Boot `rng-seed`.
4. Re-check the other boards: any board without a hardware RNG driver has the
   same race, just with different odds.

## Todos

- [ ] Reproduce the DHCP-vs-CRNG race deterministically (a fake/slow entropy
      source in the netup tests, not the bench)
- [ ] Decide fix 1 and/or 2 in gosd-init
- [ ] Establish whether an entropy source can be given to this board at all
- [ ] Audit the other boards' kernels for a real RNG driver
