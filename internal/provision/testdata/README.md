# Fixture layout for the provisioning parser (`gosd-pctc`)

This directory holds verbatim files captured off a real boot (FAT) partition
after Raspberry Pi Imager has written its provisioning data, one subdirectory
per capture scenario. `gosd-pctc`'s parser tests run directly against these
fixtures — no synthetic/hand-written fixtures, per that bean's rule that
every extraction path is fixture-driven.

This README describes the layout the parser expects; it does not itself
contain fixtures or parser code (that's `gosd-qvoq`'s empirical half, still
open, and `gosd-pctc` respectively). See `docs/provisioning-formats.md` for
what each of these files is expected to contain and why, and bean
`gosd-qvoq` for the exact bench procedure to produce them.

## Directory naming

```
testdata/
  imager-<version>/
    <scenario>/
      <files as found on the boot partition, verbatim>
```

- `<version>` is the exact Raspberry Pi Imager version used to produce the
  capture (e.g. `imager-2.0.10`), taken from `rpi-imager --version` or the
  About dialog. Capture against more than one version if behavior is
  suspected to differ (see `docs/provisioning-formats.md` for the
  known older-vs-current differences) — each gets its own `imager-<version>/`
  directory.
- `<scenario>` is a short slug for what was filled in in the customization
  dialog, matching the acceptance criteria on `gosd-qvoq`:
  - `wifi-hostname/` — WiFi SSID+password and hostname set, nothing else
  - `hostname-only/` — hostname only, WiFi left unconfigured
  - `everything/` — every field the customization wizard exposes: WiFi
    (including hidden + country), hostname, user+password, SSH key(s),
    locale (keyboard+timezone), passwordless sudo
  - Add more scenarios as needed (e.g. `wifi-open/` for an open/no-password
    network, `wifi-nonascii-ssid/` for a non-ASCII or control-byte SSID —
    see the open question in `docs/provisioning-formats.md` §"Open
    questions for the bench") — one directory per distinct combination,
    not one directory with every combination crammed in, so a fixture test
    failure points at a specific scenario.

## File contents

Each scenario directory contains **every file Imager added or modified** on
the boot partition for that run, copied verbatim (same bytes, same
filename/case) — not just the ones relevant to WiFi/hostname. That means,
depending on `init_format` (see `docs/provisioning-formats.md` §1):

- systemd format: `firstrun.sh`, `cmdline.txt` (the version *after* Imager's
  edit, not before)
- cloud-init format: `user-data`, `network-config`, `meta-data`,
  `cmdline.txt` (after edit)
- `config.txt` if the scenario touched any config.txt-level setting

Do not hand-edit or reformat any captured file — whitespace, quoting, and
line endings are exactly what the parser has to handle, and "cleaning up"
a fixture defeats the point of capturing it. If a capture turns out to be
wrong or was produced by a buggy Imager version, delete the scenario
directory rather than editing it in place, and note why in the bean.

## A note on scenario provenance

Because "Use custom" disables Imager's customization dialog entirely for a
locally-selected `.img` (see `docs/provisioning-formats.md` §0), captures
must come from either:

- `rpi-imager-cli` with `--first-run-script`/`--cloudinit-userdata`/
  `--cloudinit-networkconfig` pointing at hand-made content, or
- a custom repository (`os_list.json` with `init_format` set) so the GUI
  wizard treats the GoSD image as a catalog entry.

Record which of these produced each scenario directory (e.g. in the bean
or a one-line note in this README when fixtures land) — it affects whether
a capture exercises Imager's real field→file generator or just its
file-placement/`cmdline.txt`-mangling logic.
