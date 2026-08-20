---
# gosd-wnyc
title: 'Security audit: batch of ten low-severity hardening items'
status: completed
type: task
priority: normal
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-20T06:47:43Z
---

**Severity: Low (batch).** Nine small hardening items found during the
adversarial audit. None is individually worth a bean; together they are one
tidy-up pass. Each has a named location and a one-line fix. Split any item
out if it grows.

## Storage / mounts

- [x] **`MS_NOEXEC` missing on user-writable data mounts.**
      `internal/blockmount/platform_linux.go:123` and
      `cmd/gosd-init/internal/boot/mounts.go:236-260` set `MS_NOSUID|MS_NODEV`
      but not `MS_NOEXEC` — while `mounts.go:41-46` *does* set it for
      `/proc`, `/sys` and configfs. `/data` is reachable from a laptop and
      exposable over USB mass storage. Nothing executes from these paths
      today, so this is consistency hardening, not a live bug.

## Ingress

- [x] **Stale "ships UNWIRED" doc comments.** `cloudflared.go:28-32` and
      `tsfunnel.go:33-38` both say nothing calls `Run` yet and that wiring is
      "a later bean". False: `cmd/gosd-init/main.go:223-249` wires and calls
      both. Misleading in exactly the files a future security reviewer opens
      first.

- [x] **gosd-tsfunnel ships untrimmed tsnet**, including dormant Tailscale SSH
      server code (`internal/build/tsfunnel.go:36-54`, no `ts_omit_ssh` tag).
      A deliberate, hardware-justified tradeoff (bean gosd-h46e — the trimmed
      build broke registration), so this is a note, not a request to change
      it: revisit if upstream makes trimming work again.

## Networking

- [x] **resolv.conf writer race.** `netup/resolvconf.go:60-79` always uses the
      fixed temp path `path + ".tmp"`. On a dual-interface board (pi-3b),
      netup's and wifiup's DHCP loops can both write concurrently. Worst case
      is last-rename-wins, and it self-heals on the next lease — a unique
      temp suffix would remove the ambiguity.

## Build / CLI

- [x] **`extbuild.Spec.Name` accepts a bare `..`.**
      `internal/extbuild/extbuild.go:113-115` blocks `/` and `\` but not `.`
      or `..`, contradicting its own stated intent ("a single path
      component"). Currently inert — `filepath.Join(tmpOut, "..")` yields a
      directory and `staticelf.Verify`'s ELF read fails on it — but tighten it.

- [x] **`--publish-base-url` has no scheme validation.**
      `cmd/gosd/build.go:184-186` only checks non-empty, whereas
      `parseSupportURL` (`build.go:711-722`) enforces http(s). An `http://`
      catalog base URL publishes a plaintext download link in `os_list.json`.
      Mirror `parseSupportURL`.

- [x] **`gosd-kernel.toml` `[[firmware]]` URLs aren't required to be https.**
      `internal/kernelconfig/config.go:213-237` validates sha256 and `dest`
      but not scheme. Developer-authored file, so low impact, but inconsistent
      with the in-repo board manifests, which are all https.

## js/ package

- [x] **No warning for an unpinned `http://` manifest URL.** The image fetch is
      safe over http (hash-verified regardless of transport), but the
      *manifest* is the root of trust. Warn when it is fetched over plain HTTP
      with no `manifestSha256` pin.

- [x] **README doesn't state that escaping placeholder content is the caller's
      job.** A placeholder is a whole pre-rendered file, so there is no
      injection vector inside the library — but the quickstart's
      `renderConfigYaml(userInput)` invites an integrator to interpolate raw
      user input into YAML unescaped.

## sound/

- [x] **`Options.Format.Channels`/`.Rate` unvalidated**, unlike `Options.Volume`.
      `sound.go:246-256` special-cases only `Channels == 0`; a negative value
      wraps at `uint32(f.Channels)` (`platform_linux.go:388-389`). Not
      memory-unsafe — the kernel's `HW_PARAMS` ioctl rejects it with EINVAL
      before a `Device` exists — but an inconsistent, confusing failure mode.

- [x] **`gadget.failApply()` discards its own unwind error**
      (`gadget/gadget.go:99-111`), acknowledged in its comment. Only reachable
      when the UDC was never bound, so lower stakes than `Close()`'s version
      (separate bean).

## Summary of Changes

All eleven items done in one pass; nothing split out or deferred. Landed with
gosd-5lz2 in the same PR.

- **`MS_NOEXEC` on data mounts.** `blockmount.Mount` (emmc/disk) and
  `boot.MountDataPartition` now mount `nosuid,nodev,noexec`, and so does
  `MountDataReadOnlyFallback`, the read-only tmpfs that stands in for `/data`
  — the whole point of the item was consistency, and leaving one of the two
  `/data` mounts out would have recreated the split. Checked first that
  nothing in the tree execs from `/data` or from an `emmc`/`disk` volume:
  gosd-init's three `exec.Command` call sites are `/app` and the two ingress
  binaries, all in the initramfs rootfs, and the deferred app-slot OTA epic
  (gosd-vxal) puts its slots on GOSD-BOOT, not `/data`, so it is unaffected
  either way. Covered by a test asserting all three flags on both `/data`
  mounts.
- **Stale "ships UNWIRED" package docs.** Rewritten in both `cloudflared.go`
  and `tsfunnel.go` to describe what main.go actually does (a PanicGuard-ed
  goroutine during StartNetworking, `resolveMode` deciding whether the tunnel
  starts). Swept the rest of both files for the same staged-rollout tense
  while there: four more "the later wiring bean" references now name
  `cloudflaredDeps`/`tsfunnelDeps`. **This overlaps bean gosd-3dzc**, which
  covers the identical stale docs and whose three todos this satisfies —
  ticked there too, and gosd-3dzc can close with this PR rather than
  duplicating the work.
- **Untrimmed tsnet: reviewed, no change** — as the item asks. The tradeoff is
  already argued in full at `internal/build/tsfunnel.go`'s `tsfunnelOpts`,
  including the "revisit if upstream makes trimming work again" caveat, so
  there was nothing to write down that wasn't already written down.
- **resolv.conf writer race.** `WriteResolvConf` now stages through
  `os.CreateTemp` (chmod 0644, since CreateTemp opens 0600) instead of a
  fixed `path + ".tmp"`, so two concurrent DHCP loops can't interleave into
  one scratch file. New test runs eight writers against one path and asserts
  each read sees exactly one writer's whole file, with no scratch file left
  behind.
- **`extbuild.Spec.Name`.** Now refuses `.` and `..` alongside separators;
  its test became a table covering all four.
- **`--publish-base-url`.** `parsePublishBaseURL` mirrors `parseSupportURL`
  (absolute http(s), host required, trimmed), the flag's help says so, and
  the resolved value is threaded to `writeCatalog` rather than read off the
  package-level var.
- **`gosd-kernel.toml` `[[firmware]]` urls** must now be `https://`, with the
  reason on the `URL` field and a note in the custom-kernels doc's schema
  block.
- **js: unpinned `http://` manifest warning.** `fetchManifest` warns unless
  the manifest is pinned; loopback is exempt, since warning on every
  `http://localhost` dev run is how a warning gets ignored.
- **js: README escaping.** New "Escaping is yours, not ours" subsection under
  the threat model, naming the quickstart's `renderConfigYaml(userInput)` as
  the place the responsibility actually sits.
- **`sound.Options.Format`.** A negative `Rate` or `Channels` is refused by
  `OpenWith`, naming the field, instead of wrapping to a huge `uint32` and
  returning the kernel's bare EINVAL.
- **`gadget.failApply`.** An unwind that genuinely fails is now appended to
  the returned error, since it means Apply's "leaves nothing behind, needs no
  Close" contract did not hold. This needed one supporting change:
  `removeConfigfsTree` no longer treats `fs.ErrNotExist` as a failure — a
  node that was never created (failApply's normal case) or already removed (a
  retried Close) is the sequence succeeding, and without that the new message
  would have fired on every ordinary Apply failure.
