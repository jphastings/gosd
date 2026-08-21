---
# gosd-5lz2
title: 'Three contained defects: js manifest buffering, gadget Close state, sound control ABI test gap'
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-20T06:47:44Z
---

**Severity: Medium (batch of three).** Three independent small defects in
different packages, each with a named location and a contained fix. Split any
one out if it turns out to be bigger than it looks.

## 1. js/: `fetchManifest` buffers the whole response before checking the pin

`js/packages/gosd/src/downloads/manifest.ts:97-112` does
`new Uint8Array(await response.arrayBuffer())` **before** comparing against
`manifestSha256`. The pin is the mechanism the README recommends for safely
using an otherwise-untrusted manifest host — but it gives no protection
against that same host streaming an unbounded body: the tab can OOM before
the hash is ever computed.

The image fetch has no such gap, which is what makes this one stand out:
`substitute.ts:193-197` rejects any byte pushing past `manifest.image.size`
before buffering or hashing.

- [x] Check `Content-Length`, or stream with a hard cap (1MiB is generous for
      a manifest), before calling `.arrayBuffer()`

## 2. gadget: `Close()` clears applied state before checking teardown errors

`gadget/gadget.go:191-210` zeroes `g.fs` and `g.udc` unconditionally, even
when the UDC-unbind write or `removeConfigfsTree` failed. The in-memory
object then claims to be clean while stale configfs/UDC state remains, and
the next `Apply()` can fail with `EBUSY`/`EEXIST` and no diagnostic trail
back to the real cause.

`examples/usbserial/main.go:46-48` compounds it with
`defer func() { _ = g.Close() }()`.

- [x] Only clear applied state on a successful teardown; keep it and return
      the error otherwise
- [x] Make the example surface the Close error rather than discarding it

## 3. sound: the control/mixer ABI structs have no independent size test

`sound/platform_linux_test.go:16-63` (`TestStructLayoutMatchesKernelABI`)
pins independently-sourced reference sizes for `pcmHWParams`/`pcmSWParams`/
`pcmXferI` (608/604, 136/104, 24/12 bytes for 64- and 32-bit) and checks the
real structs against them.

There is no equivalent for `ctlElemID`/`ctlElemList`/`ctlElemInfo`/
`ctlElemValue` in `control_linux.go`, which every `Open()` exercises through
its mandatory audibility pass, and which `SetControl`/`Mixer` use directly.
Their only guard is `control_linux.go:96-101`'s compile-time assertion
against a size formula derived by the same author as the struct — so a
mistake shared between declaration and formula compiles clean and is wrong on
the device. A struct-size mismatch against the kernel ABI is memory
corruption, not a failed call.

Confirmed the compile-time assertions do hold today: the package
cross-compiles for both `GOARCH=arm GOARM=6` and `GOARCH=arm64`. The gap is
that nothing independent would catch it if the formula were wrong.

- [x] Add `control_linux_test.go` with independently-sourced reference sizes,
      mirroring the PCM test

## Summary of Changes

All three defects fixed; none needed splitting out. Landed with gosd-wnyc in
the same PR.

1. **js manifest buffering.** `fetchManifest` now reads through
   `readCappedBody`: an oversized `Content-Length` is refused outright, and
   bytes are counted as they arrive regardless (a host willing to send an
   endless body is not one whose headers mean anything), cancelling the
   reader past `MAX_MANIFEST_BYTES` (1 MiB). Tests cover both the declared
   size and an endless `ReadableStream`. The full js gate chain was run.

2. **`gadget.Close()` state.** `g.fs`/`g.udc` are cleared only after both the
   UDC unbind and `removeConfigfsTree` succeed, so a failed Close leaves the
   Gadget marked applied — which it is. A retried Close now works, which
   needed `removeConfigfsTree` to stop reporting `fs.ErrNotExist` (the second
   attempt finds most of the tree already gone); the same change is what lets
   `failApply` report a genuine unwind failure without firing on every
   ordinary Apply failure (bean gosd-wnyc's last item). `examples/usbserial`
   prints the Close error to stderr instead of discarding it, naming what a
   stranded configfs tree costs the next restart. The fake grew a `refuse`
   map so a kernel that says no (EBUSY on a bound gadget) can be tested at
   all.

3. **sound control ABI test.** New `sound/control_linux_test.go` pins
   `ctlElemID`/`ctlElemList`/`ctlElemInfo`/`ctlElemValue` at 64/80/272/1224
   (64-bit) and 64/72/272/712 (32-bit), plus the three offsets the accessors
   index by hand and the enumerated arm's size, all laid out from the C
   declarations in `include/uapi/sound/asound.h` rather than from
   `control_linux.go`'s own formula — which was the whole point of the item.
   The two totals that differ by ABI differ because of `snd_ctl_elem_value`'s
   `long value[128]`; the `snd_ctl_elem_info` union is 128 bytes on both.
