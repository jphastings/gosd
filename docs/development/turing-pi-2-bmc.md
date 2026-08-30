# Developing against the Turing Pi 2's BMC

Operational knowledge for bringing up any node type (RK1, CM4, or a future
addition) hosted in a Turing Pi 2 — gathered across the [Turing RK1](turing-rk1.md)
and [Pi CM4](pi-cm4.md) bring-ups. This is bench/tooling knowledge, not a
GoSD design decision — nothing here belongs in the repo root CLAUDE.md.

## Two different "power" controls exist — don't confuse them

If a `sdwire` rig is also on the bench, its `power` subcommand
(`sdwire power on/off/cycle`) controls whatever the SDWire's own configured
power plugin is wired to (in this project's case, a Meross smart plug
powering the bench/dock rig) — **it has nothing to do with a Turing Pi 2
node's own power rail.** It will report success (`controlling power for
device "bench"`) and do something real, just not to the node you're
targeting. This cost a full debugging detour: `sdwire power cycle` appeared
to work, but the targeted node never actually lost power.

**Use `tpi power {on,off,reset,status} -n <1-4>` for node power, always.**
Node numbers are 1-indexed, matching the physical silkscreen labels.

## `tpi` CLI auth: pass credentials explicitly

`tpi`'s interactive credential prompt / cached-token flow can be unreliable
from a sandboxed/non-interactive shell (`Device not configured (os error 6)`
intermittently, even after re-authenticating). Passing `--user root
--password <password>` on every invocation sidesteps this entirely — it
worked reliably every time this session, while the interactive/cached-token
path didn't. Same credentials work for direct REST calls (`curl -u
root:<password> -k https://turingpi.local/api/bmc?...`) and for SSH
(`ssh root@turingpi.local`) — all three surfaces (CLI, REST, SSH) share one
login.

## Getting a node's console: `tpi uart get`, not the physical header

For CM4 nodes specifically: **the 40-pin GPIO header's UART pins (8/10) are
not connected at all** — confirmed against the
[official Turing Pi 2 spec](https://docs.turingpi.com/docs/turing-pi2-specs-and-io-ports),
which says those pins are "an additional UART in the case of the Nvidia
Jetson modules and... not connected in the case of the Raspberry Pi
Compute Module 4." A physical serial adapter wired to that header will
show nothing for a CM4 node, and it isn't a wiring mistake — those pins
genuinely carry no signal for this module type. `tpi uart get -n <N>` /
`tpi uart set -n <N> -c <text>` (via the BMC's own internal bridge) is the
only console path for a CM4 node. (RK1 nodes are different: their console
does come out on the shared GPIO block's UART2 pins — see
[the Turing RK1 development notes](turing-rk1.md).)

**`tpi uart get`'s buffer can get stuck serving stale data, and survives a
`tpi reboot`.** After running `tpi reboot` (a full BMC OS reboot) mid-session,
`tpi uart get -n <N>` kept returning byte-for-byte identical content across
many different node power cycles — confirmed by a hardware timer value
embedded in the BootROM's own banner (`stc <n>`) never changing. This
persisted through a full physical power-cycle of the whole board, not just
`tpi reboot`. **Fix: restart the `bmcd` service directly over SSH**, not the
whole BMC:

```sh
ssh root@turingpi.local
/etc/init.d/S94bmcd restart
```

`bmcd` (`/usr/bin/bmcd --config /etc/bmcd/config.yaml`) is the single
daemon behind every `tpi`/REST command — power, uart, and flash all go
through it. Restarting it is far less disruptive than `tpi reboot`, which
cuts power to every node on the board, not just the one you're debugging.

**If `tpi uart get` still shows nothing after a `bmcd` restart**, verify
independently by SSHing in and reading the raw tty directly — this bypasses
`bmcd` and the REST layer entirely, so it tells you whether the problem is
in `bmcd`'s buffering or upstream of it (the node's actual UART signal):

```sh
ssh root@turingpi.local
stty -F /dev/ttyS<N> 115200 raw -echo
cat /dev/ttyS<N>            # or: timeout unavailable on this busybox — background it and check a file instead
```

**The `/dev/ttyS*` device numbers do NOT match node numbers 1:1.** Confirmed
against [BMC-Firmware#181](https://github.com/turing-machines/BMC-Firmware/issues/181):
"`/dev/ttyS4` corresponds to node 3." Don't assume `ttyS<N>` is node `<N>`;
verify empirically instead — power on only the node you care about and read
all four `ttyS1`–`ttyS4` simultaneously; whichever shows a fresh BootROM
banner is the real mapping for that node right now. Only one of the four
tty capture streams above showed genuine silence during a node-1-only test
while the other three showed a byte of floating-line noise each — a
plausible sign that a fully idle line looks different from a genuinely
disconnected/dead one, worth another data point before trusting either
reading in isolation.

## `tpi advanced msd` (eMMC-boot workaround) can leave a node in a boot loop

For a compute-module-class node with onboard eMMC, `tpi advanced msd -n <N>`
exposes the eMMC as mass storage internally to the BMC (not bridged out to
an external host — a `diskutil list`/`lsblk` on your own machine will never
see it) so `tpi flash -n <N> --image-path <path>` can write to it directly.
**Returning to `tpi advanced normal -n <N>` plus a plain `tpi power off`/`on`
did not reliably clear the node back to a normal boot** in this session —
after writing a real (non-garbage) image this way, the node got stuck
printing `Boot mode: RPIBOOT (03)` on every subsequent boot, forever, even
across multiple full power cycles. Untested but suspected: some boot-select
GPIO strap `msd` mode sets doesn't fully release on `normal`, and needs a
genuinely cold (mains-unplugged) cycle to clear — confirm this the next
time it comes up, and update this note either way.

**Prefer the SD-card path over `tpi advanced msd` when the node has an SD
slot at all** (via an attached SDWire or similar) — it's a completely
standard host-side raw block write with none of the above uncertainty, and
lets you read the card back directly on your own machine afterward to
independently verify what actually got written (partition labels, file
contents, byte sizes) without any BMC-side machinery in the loop at all.
