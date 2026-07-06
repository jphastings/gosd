# Capture notes — imager-2.0.10

Captured by JP, 2026-07-05, on macOS, using Raspberry Pi Imager **v2.0.10**.

Provenance: the custom-repository catalog flow (`os_list.json` with
`"init_format": "cloudinit"`), pointed at a GoSD-built image, flashed through
the normal GUI customization wizard — not `rpi-imager-cli`. This exercises
Imager's real field→file generator (PBKDF2 hashing, hostname insertion,
etc.), per the "note on scenario provenance" in this directory's README.

Scenario → dialog fields filled in:

- `wifi-hostname/` — WiFi SSID "demo" + password, hostname set. Has
  `network-config`.
- `hostname-only/` — hostname only; WiFi left unconfigured in the dialog.
  No `network-config` written.
- `everything/` — every field the wizard exposes: hidden WiFi SSID
  "hidden-network" + password, user + password, SSH enabled (via
  `ssh_pwauth: true` + `runcmd: systemctl enable --now ssh`), locale
  (keyboard: gb/pc105, timezone: Europe/London), passwordless sudo is not
  distinguishable in these captures (no explicit sudo/no-password toggle
  surfaced in the generated `user-data`).

Known quirk: **`hostname` reads `fixture-one` in all three scenarios'
`user-data`**, not the scenario's own name. Imager's customization dialog
persists field values from the previous run, and the hostname field was
only changed for the first ("wifi-hostname") capture, then left as-is for
the following two runs. Treat the `hostname: fixture-one` line as an
artifact of capture order, not a per-scenario signal — the scenario
directory name is authoritative for what was being tested.

`config.txt` and `gosd.toml` are **not present** in these scenario
directories: they were byte-identical across all three captures, and
byte-identical to what GoSD's own builder renders for a pi-zero-2w build
with `--hostname fixture-test` and no WiFi flags (verified by rendering
`internal/boards/pizero2w/templates.RenderConfigTxt` and
`internal/gosdtoml.Render` directly and diffing against the captured
files). Imager left both files untouched in the cloud-init flow.

Binaries present on the captured boot partition (`kernel8.img`,
`initramfs.cpio.zst`, `bootcode.bin`, `start.elf`, `fixup.dat`) are
intentionally not committed — they're either our own build output or
third-party firmware blobs, neither of which belongs in source control per
the project's third-party-binary-blob policy.

Only 3 of the 5 scenarios listed on bean `gosd-qvoq` were captured
(`wifi-hostname`, `hostname-only`, `everything`); the two optional
scenarios (`wifi-open`, `wifi-nonascii-ssid`) were not captured in this
session.
