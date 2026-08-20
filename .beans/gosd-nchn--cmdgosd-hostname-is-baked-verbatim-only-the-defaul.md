---
# gosd-nchn
title: 'cmd/gosd: --hostname is baked verbatim — only the default path is sanitized'
status: scrapped
type: task
priority: normal
created_at: 2026-07-31T07:54:11Z
updated_at: 2026-08-20T06:27:03Z
---

Found by review sweep `gosd-fuxs` (build pipeline area), verified.

`--hostname` help text says "(default: sanitized main package name)" but
only the default runs through `naming.Sanitize` (cmd/gosd/build.go:162-165,
run.go:90-93); an explicit value is baked verbatim. Every comparable flag
validates at parse time (--env regex, --data-size, --console-baud).

**Failure scenario:** `--hostname "My Device!"` builds silently; mDNS
resolution of the invalid DNS label breaks (or sethostname rejects it) —
discovered on the bench, not at the flag. Also: `naming.Sanitize` has no
length cap, so even the default path can exceed sethostname's 64-byte
limit for a long package name.

**Fix:** validate/sanitize explicit --hostname at parse time with an
actionable error (or sanitize + log what changed), and add a 63-byte cap
to naming.Sanitize. Companion runtime bean (gosd-jeaw) hardens the
device side; do both.

## Reasons for Scrapping

Verified against current code: `cmd/gosd` has no `--hostname` flag at all — checked every `Flags()...` registration in both `build.go` and `run.go`, and grepped for `"hostname"` string literals, and neither defines one. It was removed by epic `gosd-rw6n`'s config-tree work, alongside `--env`/`--env-file`/`--wifi-ssid`/`--wifi-pass` (per this repo's locked decisions), superseded by the per-value config tree's `config/hostname` file: set via `gosd build --config-dir` overlay at build time, or a hand-edit/Imager-wizard answer at runtime.

The unsanitized-path failure mode this bean actually worried about — an invalid hostname baked in and only discovered on the bench — is also already closed, on the runtime side, by the completed companion bean `gosd-jeaw`:
- `internal/naming.Sanitize` now caps its output at `naming.MaxLength` (63 bytes, sethostname(2)'s usable limit).
- `cmd/gosd-init/internal/boot/sequence.go`'s `cardHostname`/`validHostname` reject any `config/hostname` value that isn't already in `Sanitize`'s canonical form — charset and length both — logging the rejection and keeping the previous hostname rather than ever calling `SetHostname` with something invalid. This covers the config-tree hostname regardless of whether it arrived via `--config-dir` at build time or a hand-edit/wizard at runtime, which is the scenario this bean described.
- `SetHostname` failures are non-fatal (logged, boot continues) rather than rebooting, closing the reboot-loop half of gosd-jeaw's own scope.

No residual unsanitized path exists either: `cmd/gosd`'s own baked default (`Hostname: appName` in both `build.go` and `run.go`) already runs through `naming.Sanitize` before it's baked into `config.json`.

Closing as obsolete/superseded; no code change needed for this bean specifically. The rest of this review sweep's actual fixes (gosd-j6qi, gosd-2maa, gosd-4k5k, gosd-ctkj) ship in the same PR.
