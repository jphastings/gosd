---
# gosd-06kj
title: 'Pi Zero W boot files: firmware manifest, config.txt, 43430 WiFi blobs'
status: completed
type: task
priority: normal
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-07T06:24:41Z
parent: gosd-ajpz
---

Mirror the pi-zero-2w boot-files work: manifest.json pinning the SAME raspberrypi/firmware tag (bootcode.bin/start.elf/fixup.dat are shared) + RPi-Distro/firmware-nonfree pinned commit for the brcmfmac43430-sdio blob family (Zero W uses 43430/43438 — check the repo symlinks like the 2W task did and record which aliases /lib/firmware/brcm needs). Real downloaded sha256s, no placeholders. config.txt template: like pi-zero-2w but NO arm_64bit line and kernel=kernel.img; cmdline.txt identical pattern (console=serial0,115200 quiet init=/init gosd.board=pi-zero-w). go:embed templates + render tests in internal/boards/pizerow/templates.
- [x] manifest.json with verified hashes + alias findings recorded
- [x] Templates + render tests

## Summary of Changes

- Added `build/boards/pi-zero-w/manifest.json`: pins the SAME
  `raspberrypi/firmware` tag `1.20260521` (commit
  `09267f5354d40519d82fbd2193b9e211ec304055`) as pi-zero-2w for
  `bootcode.bin`/`start.elf`/`fixup.dat` — hashes verified identical to
  pi-zero-2w's manifest (same tag, same bytes, confirmed by direct
  download). Also pins `RPi-Distro/firmware-nonfree` at the SAME commit
  (`9794282eb9f4a2de1f23b41a738926740e975d83`) for the 43430 WiFi blob
  family, with every URL fetched live and its sha256 verified independently
  (no placeholders).
- Added `internal/boards/pizerow/templates`: go:embed `text/template`
  sources for `config.txt` (no `arm_64bit` line, `kernel=kernel.img`) and
  `cmdline.txt` (`gosd.board=pi-zero-w`), mirroring pizero2w's
  `RenderConfigTxt`/`RenderCmdlineTxt` API. Unit tests assert exact locked
  output, absence of `arm_64bit`, and that cmdline.txt is a single line.
- Did not touch the board profile, `internal/boards` registry, or
  `COMPATIBILITY.md` — no board status changed (pi-zero-w still isn't
  registered/buildable), per this bean's scope; that's gosd-et0q.

## 43430 WiFi blob/alias findings (2026-07-07)

Checked `RPi-Distro/firmware-nonfree` at the same pinned commit used for
pi-zero-2w. The original Pi Zero W (`raspberrypi,model-zero-w` compatible
string) resolves via git symlinks quite differently from the Zero 2 W:

- `brcmfmac43430-sdio.raspberrypi,model-zero-w.bin` and `.clm_blob` →
  symlink to `../cypress/cyfmac43430-sdio.{bin,clm_blob}` — a
  **Cypress-branded** blob living in a sibling `cypress/` upstream
  directory, not `brcm/`. This is the SAME blob the Pi 3 Model B
  (`raspberrypi,3-model-b`) uses.
- `brcmfmac43430-sdio.raspberrypi,model-zero-w.txt` → symlink to
  `brcmfmac43430-sdio.txt`, a real file alongside the symlinks in `brcm/`.
- No `43430b0` alias exists for `model-zero-w` (checked explicitly — only
  `model-zero-2-w`, `3-model-b`, and `0-compute-module` have a b0 variant),
  confirming the original Zero W shipped a single WiFi chip revision
  (plain BCM43430) throughout production, unlike the Zero 2 W's three
  revisions.
- Crucially, the Zero W's "43430" chip ID does NOT map to the same bytes as
  the Zero 2 W's plain-43430 alias: the 2W's 43430 (no b0) alias resolves
  to `brcmfmac43436s-sdio.*` (a brcm/-native blob), while the Zero W's
  resolves to the Cypress blob in `cypress/`. Same alias-naming convention,
  different underlying bytes — a real trap for anyone assuming "43430" is
  a stable identifier for one blob across boards.
- Since gosd's initramfs can't carry symlinks (per gosd-eu2x's finding),
  the eventual board profile must materialize all three alias dest names
  as literal duplicate copies under the SAME `brcm/` destDir as the base
  files, even though upstream splits the real bytes across `brcm/` and
  `cypress/` — recorded in the manifest's `notes` field for whoever wires
  `FirmwareFiles`.

Downloaded and independently sha256-verified 6 files total: 3 shared GPU
boot firmware files (hashes match pi-zero-2w's manifest exactly) + 3 WiFi
files (`cyfmac43430-sdio.bin`, `cyfmac43430-sdio.clm_blob`,
`brcmfmac43430-sdio.txt`).
