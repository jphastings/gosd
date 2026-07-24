---
# gosd-zp9s
title: radxa-zero-3e serial unreadable at 1.5M on CP210x adapters
status: in-progress
type: bug
priority: normal
created_at: 2026-07-24T15:53:48Z
updated_at: 2026-07-24T16:16:28Z
---

Found during Zero 3E bring-up (gosd-nlzf, 2026-07-24, ~2h of bench debugging). At the standard 1500000 baud console, RK3566 UART2 TX output garbles on a CP2102N adapter: slow rising edges as received (bytes skew high-bits-set), while the same adapter/wires/capture read rock-4se (RK3399) and nanopi-zero2 (RK3528) perfectly at 1.5M. Reproduced on two Zero 3E units and multiple OSes (GoSD + Armbian) — not board-specific damage. Radxa's own serial doc warns 'CP210X and PL2303x some products have baud rate limitations' and recommends CH340 cables; CP210x is likely the most common adapter our users own.

Candidate fixes to evaluate in this bean:
1. Raise uart2m0 TX drive strength via our kernel DTS patch set (rockchip pinctrl drive-strength property) — verify on hardware with a scope or empirically with the CP2102N; artifact release dance applies.
2. And/or make the console baud a build-time option (e.g. gosd build --console-baud=115200) writing extlinux.conf accordingly — proven workaround: hand-editing console=ttyS2,115200n8 on GOSD-BOOT gives 100% readable kernel-onward output with no reflash (U-Boot's compiled-in 1.5M remains unreadable; changing CONFIG_BAUDRATE would cover that but changes behavior for CH340/FTDI users who'd be fine at 1.5M).
3. At minimum: document the adapter caveat + the extlinux workaround in the board's docs row / COMPATIBILITY footnote (CH340 recommended for Zero 3E serial, per Radxa).

## Summary of Changes

Scope for this PR: items 2 and 3 only. Item 1 (uart2m0 drive-strength DTS
patch) is **deliberately deferred** to a bench session — see "Item 1
analysis" below. This bean stays **in-progress**, not completed, because of
that open item.

### Item 2: `gosd build --console-baud <rate>`

- `boards.BuildConfig` gained a `ConsoleBaud int` field (0 = unset, keep the
  board's own default) and `boards.Board` gained a `ConsoleBaudSupport()
  ConsoleBaudSupport` capability method, mirroring `UsbGadgetSupport`'s
  shape exactly.
- All three Rockchip boards (radxa-zero-3e, rock-4se, nanopi-zero2) now
  render extlinux.conf's `console=ttySN,<rate>n8` from
  `ExtlinuxConfData.ConsoleBaud`, defaulting to each board's existing 1500000
  when unset. Both Pi boards (pi-zero-2w, pi-zero-w) render cmdline.txt's
  `console=serial0,<rate>` from `CmdlineTxtData.ConsoleBaud`, defaulting to
  115200. In every case only the numeric rate changes; the UART device name
  (ttyS2/ttyS0/serial0) is untouched.
- qemu-virt's `ConsoleBaudSupport()` returns `Supported: false`: its console
  is a fixed `qemu-system-aarch64 -append "console=ttyAMA0"` argument with no
  baud rate at all (internal/qemurun), so there's nothing for the flag to
  change. `cmd/gosd/build.go`'s new `validateConsoleBaud` (same shape as
  `validateUsbGadget`) fails the build actionably if `--console-baud` is
  combined with `--board=qemu-virt`, naming the board and any other selected
  boards that do support it, rather than silently doing nothing.
- `validateConsoleBaudRate` validates the raw flag value: negative is a hard
  error; 0 (default) means "not passed"; any other positive integer is
  accepted, with a warning (not a failure) printed to stderr when it's
  outside a common-rates set (9600...3000000). Rationale: this flag exists
  specifically to work around adapters GoSD can't enumerate in advance, so
  erring permissive-with-warning avoided blocking a genuine (if unusual)
  need, while still catching likely typos.
- Documented in `gosd build --help` (flag description) and in a new
  `docs/runtime.md` "Serial console baud rate (`--console-baud`)" section
  (default rates per board family, the qemu-virt exception, the U-Boot
  caveat, and the no-reflash hand-edit alternative).
- Tests: template-render tests for the new `ConsoleBaud` field on every
  board's `templates` package (including a new `rock4se/templates/
  templates_test.go`, which didn't exist before); `board_test.go` coverage
  per board for the default-vs-override rate and `ConsoleBaudSupport()`;
  `cmd/gosd/build_test.go` coverage for both validators (unset/negative/
  common/uncommon rates; capable/incapable/mixed board selections); two
  `cmd/gosd/build_integration_test.go` acceptance tests building real
  (fake-artifact) images for radxa-zero-3e and pi-zero-2w and reading the
  overridden `console=` back out of the assembled image, plus one asserting
  `--console-baud` + `--board=qemu-virt` fails actionably.

### Item 3: documentation

- `docs/runtime.md`: new section as above.
- `COMPATIBILITY.md`: added `[^radxa-serial]` footnote (CP210x/PL2303
  garbling, Radxa's CH340 recommendation, both no-reflash workarounds, the
  U-Boot-stays-at-1500000 caveat, and a pointer to this bean for the
  still-open drive-strength fix), referenced from the Radxa Zero 3E cell of
  the "Image build via `gosd build`" row. No status cells (✅/❌/➖) changed.

### Item 1 analysis (deferred to a bench session)

Not implemented in this PR — needs a scope with real hardware to verify
signal integrity, plus the artifact-release dance (a kernel DTS patch change
only reaches real, non-`--artifacts-dir` builds after a new
`artifacts/vX.Y.Z` tag). Head start for whoever picks this up:

- **Where**: the Radxa Zero 3E's UART2 pinctrl node
  (`build/boards/radxa-zero-3e/kernel/patches/`), the same DTS-patch
  mechanism already used for this board's I2C/SPI enablement (per
  CLAUDE.md's "Peripheral enablement is per-SoC" locked decision). The RK3566
  TRM exposes `drive-strength` as a per-pin `rockchip,pins` property inside a
  `pinctrl` subnode; uart2's TX pin (`uart2m0-xfer` or equivalent bank/mux
  group depending on which uart2 IOMUX is pinned — confirm against the
  board's actual pinmux selection in the existing DTS before patching) is
  where the property would go.
- **What value**: RK3566 pinctrl drive-strength is typically encoded as an
  enum (commonly 0-3, corresponding to roughly 2mA/4mA/8mA/12mA depending on
  the specific IP block/voltage domain — verify against the RK3566 TRM's
  pinctrl chapter and the vendor's own overlay examples rather than assuming
  the common Rockchip default numbering, since some bus/voltage domains use
  different level counts). Start one or two steps above whatever the
  mainline DTS's default/unset value effectively is (unset typically means
  the IP block's own reset default, often the weakest setting) and verify
  empirically — the bug report's own working comparison boards (ROCK 4SE
  RK3399, NanoPi Zero2 RK3528) are different SoCs with different pinctrl
  IP, so their drive strength isn't a directly transferable number, only
  evidence that higher drive strength on some Rockchip part reads cleanly
  on CP210x at 1.5M.
- **Verification**: bean gosd-nlzf's bench setup (CP2102N adapter, two Zero
  3E units) is the natural place to re-test — either a scope capture of
  rise/fall time on the TX line before/after, or simply confirming clean
  `tio`/`screen` capture at 1500000 baud post-patch. Standard artifact-bump
  three-way verification applies once a value is chosen (clean-machine
  build, offline re-run, content spot-check via `dtc -I dtb -O dts` showing
  the new drive-strength property) per CLAUDE.md.
