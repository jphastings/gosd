---
# gosd-yx94
title: 'gosd-init: DHCP can fail permanently when the kernel CRNG is unseeded (cubie-a5e has no entropy source)'
status: todo
type: bug
priority: normal
created_at: 2026-08-16T19:25:39Z
updated_at: 2026-08-16T21:37:32Z
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

- [x] Reproduce the DHCP-vs-CRNG race deterministically (a fake/slow entropy
      source in the netup tests, not the bench)
- [x] Decide fix 1 and/or 2 in gosd-init
- [ ] Establish whether an entropy source can be given to this board at all
- [ ] Audit the other boards' kernels for a real RNG driver



## Second reproduction (2026-08-16, later the same evening)

Same board, clean boot of a stock `examples/hello` image, no diagnostic app
involved this time:

```
[ 23.633] gosd hello: listening on [::]:80
[146.706] [gosd] DHCP discovery on eth0 failed: DHCP discover/request on eth0:
unable to receive an offer: unable to create a discovery request: could not get
random number: context deadline exceeded; retrying in 683.069466ms
```

Identical shape to the first: **one** log line, a claimed retry, then silence —
and the board never reached the network (mDNS unresolvable and unreachable for
the remaining ~3 minutes of the capture). So this is not a one-off.

Two details worth keeping:

- The failure surfaces **~2.4 minutes after the app starts** (146s here, 123s
  the first time), long after the link is up in the boots that work — so
  whatever is retrying is doing so silently for minutes before it says
  anything, and then says it once.
- On the boots that succeed, the lease arrives ~20s in
  (`[gosd] eth0: lease {...}`) and mDNS answers immediately after. There is no
  in-between state observed: a boot either gets on the network quickly or
  never does.

Rough rate on this board: at least 2 failures across roughly 10 observed boots.
That is frequent enough that a user would meet it, and — because the app itself
starts fine and logs nothing — it presents as "my board is on but I can't reach
it", with no clue on the console unless someone is watching serial at the
2.5-minute mark.

## Summary of Changes

**Fix 1 (DHCP no longer depends on a seeded CRNG).** Traced the failure to
`github.com/insomniacslk/dhcp/dhcpv4.GenerateTransactionIDWithContext`
(`dhcpv4packet.go`), which every `dhcpv4.New*` call — including nclient4's
Discover/Request, unreachably deep inside `dhcpv4.NewDiscovery` — calls before
any caller-supplied modifier runs. Neither `nclient4.ClientOpt` nor
`dhcpv4.Modifier` exposes a way to influence it; the one real seam either
library offers is `github.com/u-root/uio/rand.Reader`, a package-level,
exported, mutable `ContextReader` that `GenerateTransactionIDWithContext`
reads from every call. On Linux its default (`getrandomReader`) busy-loops on
`getrandom(2, GRND_NONBLOCK)` until the CRNG is seeded or the library's own
2-minute `RandomTimeout` fires — exactly the observed ~123s/146s failures and
the "could not get random number: context deadline exceeded" text. Verified
`github.com/u-root/uio/rand` is imported nowhere else in gosd-init's build
graph, so overriding it is scoped to exactly this one call site.

Added `netup.dhcpXIDSource` (`dhcprand.go`), a `ContextReader` that fills
requested bytes from `math/rand/v2`'s top-level generator — seeded
non-blockingly by the Go runtime at process start (ELF auxv `AT_RANDOM`), never
touching `crypto/rand`/`/dev/urandom`/`getrandom(2)`. A DHCP transaction ID only
needs to be probably-unique among concurrent exchanges (RFC 2131 §4.1), not
cryptographic; DHCP has no authentication for anything else in the exchange
either. Wired in via a package `init()` in `platform_linux.go` (the only file
that imports the real DHCP client), so it's installed before gosd-init's first
DHCP call without any startup-path wiring.

**Fix 2 (persistent failure stays visible without spamming).** Added
`netup.retryStatus` (`dhcpstatus.go`), used by `RunDHCP` (`dhcp.go`): the first
discovery failure in a streak still logs immediately (unchanged wording);
later failures are silent until a backing-off interval elapses (starts at 10s,
doubles up to a 5-minute cap), at which point one status line reports how long
discovery has been failing and the last error. Success resets the streak.

**Tests** (`dhcprand_test.go`, `dhcpstatus_test.go`, plus one addition to
`dhcp_test.go`): `TestUnseededCRNGReproducesTheObservedDHCPFailure` installs a
fake reader that only ever blocks until `ctx` is cancelled (modeling the
unseeded-CRNG behavior exactly) and calls the real
`dhcpv4.GenerateTransactionIDWithContext`, reproducing the bean's precise
"context deadline exceeded" error deterministically, no hardware or timing
race needed. `TestDHCPXIDSourceFixesTheUnseededCRNGRace` runs the identical
call with `dhcpXIDSource` installed and a context with zero time left,
proving the fix succeeds regardless. `TestRunDHCPKeepsPersistentFailureVisibleWithoutSpamming`
drives `RunDHCP` through 8 consecutive failures via a fake clock and asserts
exactly 3 log lines (first + two backed-off status reports, each naming
elapsed time) rather than 8. `retryStatus` itself has direct unit tests for
suppression, re-reporting, interval doubling/capping, and reset.

Left unchecked: whether the board can be given a real entropy source, and
auditing other boards' kernels for hardware RNGs — both are kernel/DTS work,
not this change's job.
