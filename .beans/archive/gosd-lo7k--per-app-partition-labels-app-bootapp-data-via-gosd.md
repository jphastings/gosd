---
# gosd-lo7k
title: 'Per-app partition labels: <app>-boot/<app>-data via gosd build --label-prefix'
status: completed
type: feature
priority: normal
created_at: 2026-08-09T10:07:01Z
updated_at: 2026-08-21T01:36:07Z
---

Replace the fixed GOSD-BOOT/GOSD-DATA volume labels with per-app ones: an app
called atfs gets atfs-boot and atfs-data. When a flashed card is plugged into
a computer, the drive that appears is named after the app, not after GoSD.

## Why

The volume label is the one part of the on-card layout an end user actually
sees (docs/flashing.md tells them to look for the drive by name). Naming it
after the app makes the card self-describing. Exploration confirmed labels are
load-bearing in exactly one runtime place — dataexpand's survivor-adoption and
established-partition gates — and that no filesystem in the stack forces
uppercase (go-diskfs FAT32, the exFAT writer, and ext4 are all case-preserving
on write and read), so lowercase labels need no case-handling code.

## Locked decisions (JP, 2026-08-09)

1. **Default prefix = sanitized app name truncated to 6 bytes** (trailing `-`
   trimmed), so both labels fit FAT's 11-byte cap with the 5-byte
   `-boot`/`-data` suffixes. `sattrack` → `sattra-boot`/`sattra-data`. The
   chosen labels are printed during the build.
2. **Clean break for legacy cards — no GOSD-DATA adoption alias.** A card
   flashed by a pre-change release fails the survivor gate on its first
   reflash-upgrade and is cleanly reformatted. Release-notes-level breaking
   callout. No halt is reachable, via two distinct mechanisms (adversarial
   review 2026-08-09): an expand image's reflash drops the MBR entry, so the
   legacy volume is met by the creation path's survivor gate (reformat, not
   halt); a fixed-size image ships an MBR entry, but the same flash also
   overwrites partition 2's filesystem with one stamped with this image's
   own label, so verifyEstablished always reads a matching volume.
   Consequence recorded for phase-2 self-update (gosd-522n): both mechanisms
   rely on "installing a new image rewrites the MBR and/or partition 2" — a
   self-update payload must never change dataLabel (or must relabel and
   re-establish the volume as part of the update).
3. **Single `--label-prefix` flag** on gosd build and gosd run: labels are
   always `<prefix>-boot`/`<prefix>-data`. An explicit prefix is used verbatim
   (no sanitizing or lowercasing), validated: non-empty, ≤6 bytes, and only
   `[A-Za-z0-9_-]` (decided at adversarial review 2026-08-09 — printable
   ASCII would admit FAT 8.3-reserved punctuation like `/:*?"<>|` into the
   short-name entry the label is stored as; letters/digits/hyphen/underscore
   is simple to state and can't hit any filesystem quirk, while still
   preserving verbatim case). Both resulting labels must pass
   blockmount.ValidateLabel.
4. **No legacy constants; labels are required end-to-end.** No code path needs
   to name the old labels — a legacy survivor is rejected by *mismatch*, not
   by recognizing it. cmd/gosd always resolves a label pair, so downstream
   layers require it: empty image.Spec labels are a guarded error in Write;
   config.json always carries dataLabel (no omitempty); empty
   dataexpand.Options.DataLabel is an early actionable error. The old label
   values survive only in release notes, this bean, CLAUDE.md's amended
   decision, and the clean-break regression test's literal.
5. The pair travels as **concrete labels, not a prefix**, past the CLI;
   config.json carries only `dataLabel` (nothing on-device reads the boot
   label — boot mounting is device-node + gosd.toml-sentinel based).
6. dataexpand's label comparisons become **EqualFold** (matching blockmount's
   labelMatches), so an external tool relabeling to uppercase can't trigger a
   spurious reformat or halt.
7. `--label-prefix` does **not** move image Identity (config.json is excluded
   from ComputeIdentity; consistent with --boot-size/--data-flush), pinned by
   a test.
8. **The label is on-card ABI** like --boot-size: changing the prefix (or
   renaming the app) between releases reformats the data partition on
   reflash-upgrade. Cross-app reflash stops silently inheriting the previous
   app's data. Documented in flag help, upgrade-path.md, CLAUDE.md.
9. Bugfix folded in: `gosd run` derives the app name inline (breaks for
   `gosd run .`); switch it to build.go's deriveAppName.

## Mechanism notes

- Plumbing mirrors --data-filesystem (bean gosd-95yu, PR #242) exactly:
  flag → pipeline.Options → image.Spec + initcfg.Config → boot sequence →
  dataexpand.Options. The label flows into both dataexpand format routes
  (FormatFAT32 and FormatEXT4) and both gates (survivorPresent,
  verifyEstablished).
- The 6-byte prefix cap stays keyed on FAT's 11-byte limit regardless of
  --data-filesystem: the boot label is always FAT32.
- blockmount/disk/emmc, js/, catalog, gosdtoml schema, board templates, and
  the provisioning snapshot need no logic changes (verified label-free).
- Also fixes the stale "FAT stores labels upper-cased" claim in
  internal/blockmount/blockmount.go's mount-only-decision comment.

## Todos

- [x] internal/naming: LabelPrefix/LabelsFor + PartitionLabels, tests
- [x] internal/image: Spec.BootLabel/DataLabel required, guard in Write, tests
- [x] internal/initcfg: Config.DataLabel (always written), tests
- [x] dataexpand: Options.DataLabel required, EqualFold gates, both format
      routes stamp it, clean-break + case-insensitivity tests
- [x] boot sequence + main.go: thread the label, halt message names it,
      label-free rewording where gosd-init doesn't know a label
- [x] pipeline: Options.Labels passthrough to image.Spec + initcfg.Config
- [x] cmd/gosd: labels.go resolveLabels, --label-prefix on build and run,
      run.go deriveAppName fix, parity test, integration tests (labels read
      back from image + config.json; Identity unaffected)
- [x] Docs sweep (flashing, runtime, upgrade-path, ab-updates, ingress,
      image-injection, publishing, gosd.toml, provisioning-formats,
      externals, examples), CLAUDE.md naming-surfaces + layout-ABI
      amendments, release-notes breaking entry
- [x] CI qemu-expand-data: add a grep proving the configured label reaches a
      real on-device format ("as FAT32 labelled hello-data"; mirrored in
      qemu-data-ext4 with "as ext4 labelled hello-data")
- [x] Adversarial review pass on the adoption-gate changes before requesting
      JP's review — full boot-scenario trace, no blockers; its 6 should-fixes
      and 6 nits are all applied (safety-argument comment states both no-halt
      mechanisms; self-update dataLabel constraint recorded here, in
      dataexpand's package doc and ab-updates.md; empty-label guard in
      pipeline.Assemble; parity test genuinely asserts label parity;
      fixed-size halt recovery text corrected; don't-rename warning;
      explicit-prefix charset tightened to [A-Za-z0-9_-])

## Summary of Changes

- `internal/naming/labels.go` (new): `LabelPrefixMaxLength = 6`,
  `BootLabelSuffix`/`DataLabelSuffix`, `PartitionLabels{Boot, Data}`,
  `LabelPrefix` (Sanitize → truncate to 6 bytes → trim trailing `-` → `app`
  fallback), `LabelsFor`. Fixed the stale "FAT stores labels upper-cased"
  claim in `internal/blockmount` (this stack is case-preserving on write and
  read; the case-insensitive matching remains the load-bearing part).
- `internal/image`: label consts deleted; `Spec.BootLabel`/`Spec.DataLabel`
  required, guarded (non-empty, ≤11 bytes) before any file creation because
  go-diskfs silently truncates via `%-11.11s`; both FAT32 CreateFilesystem
  sites and the ext4 golden write stamp the Spec labels.
- `internal/initcfg`: `Config.DataLabel` (json `dataLabel`, no omitempty —
  the pipeline always writes it), documented as on-card ABI beside
  `DataFilesystem` and as Identity-neutral.
- `dataexpand`: `const Label` deleted; `Options.DataLabel` required with an
  early actionable error before any device touch; both gates
  (`survivorPresent`, `verifyEstablished`) compare with `strings.EqualFold`;
  both format routes stamp the configured label and log it
  ("formatting %s as FAT32/ext4 labelled %s ..."); `ErrDataCorrupt` names
  the configured label. Crash ordering (write → sync → marker → sync → MBR)
  untouched — only the compared string changed.
- `boot` + `gosd-init/main.go`: `cfg.DataLabel` threaded through
  `Deps.ExpandData`; `haltForDataCorruption` names the image's real
  filesystem and label in boot-failure.log; label-free rewording where
  gosd-init doesn't know a label (boot sentinel messages etc.).
- `pipeline`: `Options.Labels naming.PartitionLabels` threaded into
  `image.Spec` and `initcfg.Config.DataLabel` in every data-size mode.
- `cmd/gosd`: new `labels.go` (`resolveLabels` — default via
  `naming.LabelPrefix`, explicit prefix verbatim, validated ≤6 bytes /
  printable ASCII / no spaces, both labels through
  `blockmount.ValidateLabel`); `--label-prefix` on build and run with the
  chosen labels printed; `gosd run .` app-name derivation aligned with
  build's `deriveAppName` (bugfix); GOSD-* wording removed from flag help
  and errors.
- Tests: label-rule tables + ValidateLabel invariant; image round-trip incl.
  a verbatim mixed-case pair; clean-break pin (a `GOSD-DATA` survivor is
  reformatted, never adopted — the only place the old literal survives);
  EqualFold adoption in both gates; empty-label guards; config.json
  `dataLabel` read back from a built image; Identity unaffected by
  `--label-prefix`; buildrun parity with `--label-prefix`.
- CI: qemu-expand-data asserts "as FAT32 labelled hello-data" and
  qemu-data-ext4 asserts "as ext4 labelled hello-data" on their first-boot
  steps — end-to-end proof the baked label reaches a real on-device format.
- Docs: flashing/runtime/upgrade-path/ab-updates/ingress/image-injection/
  publishing/gosd.toml/provisioning-formats/externals swept; upgrade-path
  gains the label-ABI decision; UNRELEASED.md breaking entry; CLAUDE.md
  naming-surfaces + layout-ABI amendments; COMPATIBILITY.md row renamed to
  "ext4 data partition"; examples comments/HTML updated.
