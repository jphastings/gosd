---
# gosd-5lz2
title: 'Three contained defects: js manifest buffering, gadget Close state, sound control ABI test gap'
status: todo
type: bug
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-12T04:18:42Z
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

- [ ] Check `Content-Length`, or stream with a hard cap (1MiB is generous for
      a manifest), before calling `.arrayBuffer()`

## 2. gadget: `Close()` clears applied state before checking teardown errors

`gadget/gadget.go:191-210` zeroes `g.fs` and `g.udc` unconditionally, even
when the UDC-unbind write or `removeConfigfsTree` failed. The in-memory
object then claims to be clean while stale configfs/UDC state remains, and
the next `Apply()` can fail with `EBUSY`/`EEXIST` and no diagnostic trail
back to the real cause.

`examples/usbserial/main.go:46-48` compounds it with
`defer func() { _ = g.Close() }()`.

- [ ] Only clear applied state on a successful teardown; keep it and return
      the error otherwise
- [ ] Make the example surface the Close error rather than discarding it

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

- [ ] Add `control_linux_test.go` with independently-sourced reference sizes,
      mirroring the PCM test
