---
# gosd-fkkr
title: 'gosd-init: any panic permanently bricks the board (no panic= on cmdline, PANIC_TIMEOUT=0, no recover, PID-1 may return)'
status: completed
type: bug
priority: high
created_at: 2026-07-31T07:51:53Z
updated_at: 2026-07-31T07:51:53Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

Three reinforcing gaps convert any latent bug in gosd-init or its
third-party parsers (pion/mdns, insomniacslk/dhcp, yaml) into a permanent
brick instead of a 5-second reboot:

- No board's cmdline/extlinux template carries `panic=` (verified: e.g.
  internal/boards/pi3b/templates/cmdline.txt.tmpl is just
  `console=... quiet init=/init gosd.board=...`).
- Every recorded kernel.config has `CONFIG_PANIC_TIMEOUT=0` (e.g.
  build/boards/pi-3b/kernel.config:7414), so `Attempted to kill init!`
  spins forever.
- No `recover()` anywhere in cmd/gosd-init; `main()` ends with
  `_ = boot.Run(...)` and returns — a panic in any goroutine (or boot.Run
  returning) exits PID 1.

**Failure scenario:** a malformed multicast packet panics pion/mdns (or any
nil-map/index bug anywhere) → PID 1 dies → kernel panics with timeout 0 →
unattended appliance is dead until physically power-cycled; nothing reaches
boot-failure.log.

**Fix (two independent belts):** (a) append `panic=10` to every board's
cmdline/extlinux template — Pi templates need no artifact release; (b)
recover-wrap each long-running goroutine started from main/StartNetworking
and the supervise loop (log + reboot), and end main with an explicit
sync+reboot+select{} so PID 1 can never simply return. Note the Rockchip
extlinux templates ship in the image (templates, not artifacts), so (a) is
image-side for all boards.

## Summary of Changes

Both belts are in, image-side only — no kernel config was touched, so no
artifacts release is needed: `panic=` on the command line overrides
`CONFIG_PANIC_TIMEOUT` at runtime.

**Belt (a) — `panic=10` on every board's kernel command line.** Appended to
`internal/boards/{pi3b,pizero2w,pizerow}/templates/cmdline.txt.tmpl` and
`internal/boards/{radxazero3e,rock4se,nanopizero2}/templates/extlinux.conf.tmpl`,
and to qemu-virt's `-append`, which is built in `internal/qemurun`'s `Args`
rather than from a template. Each board's template test (and the three
`cmd/gosd/build_integration_test.go` extlinux assertions that read the file
back out of a built image) asserts the exact line, so the token is pinned
for every board on both the render and the built-image paths.

**Belt (b) — gosd-init is panic-safe as PID 1.** New
`cmd/gosd-init/internal/boot/panicguard.go` holds `PanicGuard`: `Go`/`Guard`
run work with a `recover()` that logs `panic in <name>: <value>` plus
`debug.Stack()` to the console, then syncs, waits 5s so the trace is
readable on a serial console, and reboots through the existing `Rebooter`
seam. It's pure logic over that seam, so it's fake-tested on macOS like the
rest of the package. Wired at every long-running goroutine gosd-init itself
starts: `netup`, `timesync`, the mDNS responder and `wifiup` (main.go's
`StartNetworking`), the `StartNetworking` dispatch and the supervise loop
(`sequence.go`). `main()` can no longer return: it calls the new
`boot.RunAndReboot`, which reboots however the sequence ends — fatal path,
panic, or a clean return — and then blocks in `select{}`.

Residual gap, deliberate: goroutines started *inside* third-party libraries
(pion/mdns's own readers, say) can't be recovered from any other goroutine
in Go. Those are exactly what belt (a) covers — a 10s reboot instead of a
dead board.

Tests added (`cmd/gosd-init/internal/boot/panicguard_test.go`): a panicking
guarded call logs the stack and reboots after a 5s pause; a panicking
guarded *goroutine* reboots (fake `Rebooter` signals on `Reboot`); a
non-panicking call is untouched; a panic thrown from `StartNetworking`
reboots via `Run`; `RunAndReboot` reboots both when the sequence returns
cleanly and when it panics outright.

Verified: `go test ./...` (plus `-race -count=3` over `cmd/gosd-init/...`,
since the guards log and reboot from their own goroutines), `go vet ./...`,
`gofmt -l .` clean, `golangci-lint run ./...` and
`GOOS=linux golangci-lint run ./...` both 0 issues. `docs/runtime.md`'s
supervision section now states the policy.
