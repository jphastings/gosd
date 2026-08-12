---
# gosd-wnyc
title: 'Security audit: batch of ten low-severity hardening items'
status: todo
type: task
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-12T04:18:42Z
---

**Severity: Low (batch).** Nine small hardening items found during the
adversarial audit. None is individually worth a bean; together they are one
tidy-up pass. Each has a named location and a one-line fix. Split any item
out if it grows.

## Storage / mounts

- [ ] **`MS_NOEXEC` missing on user-writable data mounts.**
      `internal/blockmount/platform_linux.go:123` and
      `cmd/gosd-init/internal/boot/mounts.go:236-260` set `MS_NOSUID|MS_NODEV`
      but not `MS_NOEXEC` — while `mounts.go:41-46` *does* set it for
      `/proc`, `/sys` and configfs. `/data` is reachable from a laptop and
      exposable over USB mass storage. Nothing executes from these paths
      today, so this is consistency hardening, not a live bug.

## Ingress

- [ ] **Stale "ships UNWIRED" doc comments.** `cloudflared.go:28-32` and
      `tsfunnel.go:33-38` both say nothing calls `Run` yet and that wiring is
      "a later bean". False: `cmd/gosd-init/main.go:223-249` wires and calls
      both. Misleading in exactly the files a future security reviewer opens
      first.

- [ ] **gosd-tsfunnel ships untrimmed tsnet**, including dormant Tailscale SSH
      server code (`internal/build/tsfunnel.go:36-54`, no `ts_omit_ssh` tag).
      A deliberate, hardware-justified tradeoff (bean gosd-h46e — the trimmed
      build broke registration), so this is a note, not a request to change
      it: revisit if upstream makes trimming work again.

## Networking

- [ ] **resolv.conf writer race.** `netup/resolvconf.go:60-79` always uses the
      fixed temp path `path + ".tmp"`. On a dual-interface board (pi-3b),
      netup's and wifiup's DHCP loops can both write concurrently. Worst case
      is last-rename-wins, and it self-heals on the next lease — a unique
      temp suffix would remove the ambiguity.

## Build / CLI

- [ ] **`extbuild.Spec.Name` accepts a bare `..`.**
      `internal/extbuild/extbuild.go:113-115` blocks `/` and `\` but not `.`
      or `..`, contradicting its own stated intent ("a single path
      component"). Currently inert — `filepath.Join(tmpOut, "..")` yields a
      directory and `staticelf.Verify`'s ELF read fails on it — but tighten it.

- [ ] **`--publish-base-url` has no scheme validation.**
      `cmd/gosd/build.go:184-186` only checks non-empty, whereas
      `parseSupportURL` (`build.go:711-722`) enforces http(s). An `http://`
      catalog base URL publishes a plaintext download link in `os_list.json`.
      Mirror `parseSupportURL`.

- [ ] **`gosd-kernel.toml` `[[firmware]]` URLs aren't required to be https.**
      `internal/kernelconfig/config.go:213-237` validates sha256 and `dest`
      but not scheme. Developer-authored file, so low impact, but inconsistent
      with the in-repo board manifests, which are all https.

## js/ package

- [ ] **No warning for an unpinned `http://` manifest URL.** The image fetch is
      safe over http (hash-verified regardless of transport), but the
      *manifest* is the root of trust. Warn when it is fetched over plain HTTP
      with no `manifestSha256` pin.

- [ ] **README doesn't state that escaping placeholder content is the caller's
      job.** A placeholder is a whole pre-rendered file, so there is no
      injection vector inside the library — but the quickstart's
      `renderConfigYaml(userInput)` invites an integrator to interpolate raw
      user input into YAML unescaped.

## sound/

- [ ] **`Options.Format.Channels`/`.Rate` unvalidated**, unlike `Options.Volume`.
      `sound.go:246-256` special-cases only `Channels == 0`; a negative value
      wraps at `uint32(f.Channels)` (`platform_linux.go:388-389`). Not
      memory-unsafe — the kernel's `HW_PARAMS` ioctl rejects it with EINVAL
      before a `Device` exists — but an inconsistent, confusing failure mode.

- [ ] **`gadget.failApply()` discards its own unwind error**
      (`gadget/gadget.go:99-111`), acknowledged in its comment. Only reachable
      when the UDC was never bound, so lower stakes than `Close()`'s version
      (separate bean).
