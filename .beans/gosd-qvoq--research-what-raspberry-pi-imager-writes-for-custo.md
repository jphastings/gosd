---
# gosd-qvoq
title: 'Research: what Raspberry Pi Imager writes for custom images (with captured fixtures)'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:07:10Z
updated_at: 2026-07-06T00:35:52Z
parent: gosd-b22t
---

Settle exactly what provisioning data Raspberry Pi Imager leaves on the boot partition when a user flashes a CUSTOM image (Use custom) with OS customization filled in (WiFi SSID/password, hostname, user, locale). Everything downstream depends on this being empirical, not assumed.

Method:
- [x] Install the current Raspberry Pi Imager release (source analysis used v2.0.10 — confirm the bench version matches or note the delta); the CLI binary (`rpi-imager-cli`) ships in the same download/package as the GUI on macOS/Linux/Windows — bench version confirmed v2.0.10, matches source analysis exactly
- [x] IMPORTANT, source-confirmed: selecting "Use custom" → a local .img in the GUI disables the OS customization dialog entirely (see docs/provisioning-formats.md §0) — do NOT expect to flip on WiFi/hostname fields that way, the step won't appear. Use one of the two paths below instead. Confirmed at the bench: Path B (custom-repo catalog) was used, exactly as this note prescribes.
  - [ ] Path A (quick, validates file placement/cmdline.txt only, not the real field→file generator): build a dummy .img with a FAT32 first partition (use our image writer), then run `rpi-imager-cli --first-run-script <file> <dummy.img> <dst>` (systemd format) and separately `--cloudinit-userdata <file> --cloudinit-networkconfig <file>` (cloud-init format) against the same dummy image, with hand-made content. Read back every file on the resulting boot partition. NOT exercised — Path B (below) was used instead and satisfies this bean's acceptance criteria; left unchecked as this alternate path genuinely was not run.
  - [x] Path B (preferred, exercises the real GUI wizard): host a minimal os_list.json (schema: doc/json-schema/os-list-schema.json in raspberrypi/rpi-imager) that lists the dummy .img with "init_format": "systemd" (repeat with "cloudinit" if time allows), point Imager's Settings → Custom repository at it, then flash through the normal GUI wizard with the customization dialog filled in for each scenario below. This is the scenario that matters for GoSD end users. Done: custom-repo catalog with init_format=cloudinit, real GUI wizard, macOS, v2.0.10.
  - [x] Scenarios to run (each its own capture): (1) WiFi SSID+password and hostname set, nothing else; (2) hostname only, WiFi left unconfigured; (3) everything the wizard exposes — WiFi (incl. hidden + country), hostname, user+password, SSH key(s), keyboard+timezone, passwordless sudo. Optional if time allows: (4) open/no-password WiFi network; (5) WiFi SSID containing non-ASCII or control-byte characters (validates the \xHH-escaping / ssid=hex: paths flagged as a genuinely open question in docs/provisioning-formats.md). Required scenarios (1)-(3) captured. Optional (4) and (5) were NOT captured in this session — remain open.
  - [x] For each scenario, copy off the FAT partition every file Imager added or changed verbatim: firstrun.sh, cmdline.txt (post-edit), and/or user-data + network-config + meta-data, plus config.txt if touched — see internal/provision/testdata/README.md for the exact directory layout and naming (imager-<version>/<scenario>/...). config.txt and gosd.toml were confirmed byte-identical to our own builder's output across all three captures (diffed against internal/boards/pizero2w/templates.RenderConfigTxt and internal/gosdtoml.Render directly) — not touched by Imager, so not committed; see capture-notes.md.
- [x] Read the rpi-imager source (github.com/raspberrypi/rpi-imager, OS customization code) to confirm which mechanism applies to custom images and when it chooses cloud-init (user-data/network-config), firstrun.sh + cmdline.txt edit, or custom.toml — and whether the WiFi PSK is written plaintext or PBKDF2-hashed
- [x] Also check: does Imager behave differently if the image has no cmdline.txt (Radxa images)? Does it corrupt anything we need?
- [x] Commit every captured file verbatim as `internal/provision/testdata/imager-<version>/<scenario>/...` — committed under `internal/provision/testdata/imager-2.0.10/{wifi-hostname,hostname-only,everything}/`
- [x] Write docs/provisioning-formats.md: formats found, field-by-field extraction table (SSID, PSK+hash format, hostname, user), version differences, and a recommendation for parser precedence

## Acceptance
Fixtures committed for at least 3 scenarios from a real Imager run; doc answers plaintext-vs-hashed PSK and the format-selection question with source links.

## Source-analysis findings summary

Full detail and citations in `docs/provisioning-formats.md`. Headline
findings:

1. **"Use custom" + local .img disables Imager's OS customization dialog
   entirely**, in every version checked (as far back as v1.7.5) —
   `imageSupportsCustomization()` is just `!_initFormat.isEmpty()`, and
   `_initFormat` is only ever populated from a catalog (`os_list.json`)
   entry's `init_format` field, never for a locally-browsed file. This means
   the naive "flash a GoSD image via Imager's gear icon" flow does not
   exist as a GUI path. The two real paths are `rpi-imager-cli` with
   `--first-run-script`/`--cloudinit-*` flags, or publishing a custom
   `os_list.json` repo (`ImageWriter::setCustomRepo`) with `init_format`
   set so the GUI treats the GoSD image as a normal catalog entry. This
   affects parent epic `gosd-b22t`'s assumptions and should be considered
   there.
2. **Mechanism selection**: `init_format` is one of `"systemd"` (writes
   `firstrun.sh` + edits `cmdline.txt`), `"cloudinit"`/`"cloudinit-rpi"`
   (writes `user-data`/`network-config`/`meta-data` + edits `cmdline.txt`),
   or empty (no customization). **`custom.toml` does not exist anywhere in
   the rpi-imager source** — it's not a real mechanism. `firstrun.sh` was
   the only, universal, ungated mechanism before v1.7.x; cloud-init was
   added to Imager well before Raspberry Pi OS itself gained native
   cloud-init support (RPi OS: Trixie, 24 Nov 2025). Given that timeline,
   the large majority of Raspberry Pi OS images in the wild today are
   still on the `firstrun.sh` mechanism.
3. **WiFi PSK is always PBKDF2-hashed, never plaintext**, in every format
   and every version checked: `PBKDF2-HMAC-SHA1(passphrase, salt=SSID,
   4096 iterations, 32-byte output)`, hex-encoded, computed client-side
   the moment the user finishes typing (`WifiCustomizationStep.qml`) before
   it ever reaches a generator or file. This is byte-for-byte identical to
   gosd-init's existing `wifiup.DerivePSK` — no new hashing/parsing
   capability is needed on the GoSD side, `gosd-pctc` just extracts the
   64-hex string.
4. **No-`cmdline.txt` (Radxa) behavior: silent no-op, not a break.**
   `DeviceWrapperFatPartition::readFile` returns empty (not an error) for a
   missing file; Imager creates a brand-new `cmdline.txt` containing only
   the customization tokens, and writes `firstrun.sh` unconditionally
   alongside it. Nothing else is touched or corrupted. But since Radxa's
   U-Boot never turns that `cmdline.txt` into `/proc/cmdline`,
   `systemd.run=/boot/firstrun.sh` is never seen by systemd, so
   `firstrun.sh` never executes — it just sits inert on the boot partition.
   Upstream's own schema documentation confirms the precondition explicitly:
   "THIS WILL ONLY WORK IF THE FAT PARTITION IS MOUNTED AT /boot in your
   /etc/fstab." This is exactly why `gosd-pctc`'s plan to regex-parse
   `firstrun.sh` directly (never execute it) is the right approach — nothing
   else will ever run it for us.
5. **Recommended parser precedence** (matches what's already locked on
   `gosd-pctc`): `gosd.toml > custom.toml > cloud-init files > firstrun.sh >
   baked config.json`. Rationale for each link in the chain is in
   `docs/provisioning-formats.md` §6 — briefly: gosd.toml is explicit and
   ours; custom.toml's slot is reserved despite not being a known real
   producer; cloud-init is structured YAML (safer to parse) and the
   direction Imager/RPi OS are heading; firstrun.sh is shell that must never
   be executed and is realistically the format gosd-init will see most
   often today; baked config.json is the pre-user-intent fallback.

## Bench todos

The "Method" checklist above now has explicit, mechanical instructions for
the empirical half (still unchecked, still someone else's job per this
bean's scope): which Imager paths to use given finding #1, exactly which
scenarios to capture, and which files to copy off the card for each. See
`internal/provision/testdata/README.md` for the exact fixture directory
layout the future parser (`gosd-pctc`) expects.

## Summary of Changes

Completed the source-analysis half of this bean (the empirical
fixture-capture half remains open, hence bean stays in-progress):

- `docs/provisioning-formats.md`: format catalog (systemd/cloudinit/
  cloudinit-rpi, confirming custom.toml doesn't exist), current (v2.0.10)
  vs older (v1.6.2/v1.7.5) Imager behavior, field-by-field extraction table
  for hostname/WiFi/user/locale/etc. across firstrun.sh and cloud-init,
  the PBKDF2 WiFi PSK derivation (never plaintext), the no-cmdline.txt/Radxa
  finding, what consumes each format on a real Raspberry Pi OS boot, and a
  recommended (and already-locked-elsewhere) parser precedence with
  rationale. Every claim cites a specific rpi-imager tag/commit permalink.
- `internal/provision/testdata/README.md`: fixture directory layout
  (`imager-<version>/<scenario>/...`) the future parser bean (`gosd-pctc`)
  expects, plus scenario list and a note on GUI-vs-CLI/custom-repo
  provenance. No parser code, per this bean's scope.
- Rewrote the three empirical "Method" todos with mechanical detail (exact
  scenarios, exact capture paths given finding #1, exact files to copy) so
  a human can execute them without re-deriving the plan; left them
  unchecked.

## Summary of Changes (empirical fixture capture)

Completed the empirical half of this bean, capturing real Raspberry Pi
Imager v2.0.10 output via the custom-repository catalog flow
(`init_format: "cloudinit"`), macOS, 2026-07-05:

- Committed `internal/provision/testdata/imager-2.0.10/{wifi-hostname,
  hostname-only,everything}/`: verbatim `user-data`, `network-config`
  (where present), `meta-data`, `cmdline.txt` for all three required
  scenarios, plus a shared `capture-notes.md` documenting provenance,
  scenario-to-dialog-field mapping, and the hostname-persistence quirk
  (Imager's dialog carries over the hostname field between runs, so all
  three captures' `user-data` reads `hostname: fixture-one`; the scenario
  directory name is authoritative for what was tested, not that field).
- `config.txt` and `gosd.toml` were NOT committed: verified byte-identical
  across all three captures and byte-identical to our own builder's output
  (rendered directly via `pizero2w/templates.RenderConfigTxt` and
  `gosdtoml.Render` with `hostname="fixture-test"`, no WiFi) — Imager left
  both untouched in the cloud-init flow.
- Secrets hygiene checked before committing: WiFi PSKs are 64-hex PBKDF2
  digests (not plaintext), SSIDs are the fake `demo`/`hidden-network`, the
  user password is a `$y$` (yescrypt) hash. Nothing real was captured.
- Appended an "Empirical confirmation" section to
  `docs/provisioning-formats.md` cross-referencing the capture against
  every source-analysis claim (cloud-init trio, cmdline.txt append-not-
  replace with our `init=` tokens preserved, PBKDF2 PSK, config.txt/
  gosd.toml untouched, hostname quirk) — no surprises versus the source
  analysis.
- Not captured: the two optional scenarios (open/no-password WiFi;
  non-ASCII/control-byte SSID) and Path A (CLI-flag capture) — Path B
  (custom-repo GUI wizard) was sufficient and is the scenario that matters
  for GoSD end users, so Path A was skipped rather than run redundantly.
  These remain open if a future bean wants full coverage of the "Open
  questions for the bench" list in docs/provisioning-formats.md.

Remaining unchecked on this bean: Path A (see above, intentionally
skipped) and the two optional scenarios. Bean stays `in-progress`.
