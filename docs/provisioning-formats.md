# Provisioning formats: what Raspberry Pi Imager writes

Every claim below is cited against a specific `rpi-imager` tag/commit so it
can be re-checked as upstream changes. The last section describes what
`gosd-init` actually reads out of all this (`internal/provision`); the rest
explains what Raspberry Pi Imager itself writes to the boot partition, and
why, so that section's choices make sense.

Primary source: [github.com/raspberrypi/rpi-imager](https://github.com/raspberrypi/rpi-imager).
Current release analyzed: **v2.0.10** (commit
`467be3d3e88f5d83fa78c78788f6e6fdce61a47e`). Older behavior analyzed at
**v1.6.2** (commit `4a039b78853a87b665b7ab89d819f36af591a1b1`, pre-cloud-init,
pre-`init_format`) and **v1.7.5** (commit `b49408781a3c347bd6f6c057c68bb34d6c06ad10`,
transitional). All permalinks below point at these exact commits, not branch
tips.

## "Use custom" disables OS customization entirely

In the Imager GUI, selecting "Use custom" and browsing to a local `.img`
file disables OS customization entirely: the customization wizard step
never appears. There is no GUI path that flashes a local image with
WiFi/hostname/etc. filled in via the gear icon.

Why: customization availability is gated by
`ImageWriter::imageSupportsCustomization()`, which is just `!_initFormat.isEmpty()`
([src/imagewriter.cpp#L4082-L4085](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/imagewriter.cpp#L4082-L4085)).
`_initFormat` is populated from the `init_format` field of the *catalog*
entry (`os_list.json`) the user picked
([src/oslistmodel.cpp#L342](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/oslistmodel.cpp#L342),
consumed in
[src/wizard/OSSelectionStep.qml#L717-L730](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/wizard/OSSelectionStep.qml#L717-L730)).
The "Use custom" entry itself is synthesized with no `init_format` key at all
([src/imagewriter.cpp#L2364-L2371](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/imagewriter.cpp#L2364-L2371)),
and when the user actually picks a local file, the QML handler calls
`ImageWriterSingleton.setSrc(fileUrl)` with **no** `initFormat` argument at
all, then explicitly clears every customization flag once it observes
`customizationSupported` is false
([src/wizard/OSSelectionStep.qml#L162-L182](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/wizard/OSSelectionStep.qml#L162-L182)).
The wizard then skips the customization step outright
([src/wizard/WizardContainer.qml#L840](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/wizard/WizardContainer.qml#L840)).

There are exactly two ways to get Imager to write provisioning data onto a
GoSD image:

1. **`rpi-imager-cli`**, which always sets `_initFormat` itself
   (`"systemd"` unless `--cloudinit-userdata`/`--cloudinit-networkconfig` is
   given, forcing `"cloudinit"`) regardless of source
   ([src/cli.cpp#L120-L121](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/cli.cpp#L120-L121)),
   and accepts a pre-made firstrun.sh / user-data / network-config file on
   the command line
   ([src/cli.cpp#L216-L289](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/cli.cpp#L216-L289)).
   This only proves file *placement and cmdline.txt mangling* — the content
   is whatever's hand-crafted, not what the GUI's field-to-file generator
   produces.
2. **A custom repository** (`ImageWriter::setCustomRepo`,
   [src/imagewriter.h#L107](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/imagewriter.h#L107)):
   host a small `os_list.json` (schema documented at
   [doc/json-schema/os-list-schema.json](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/doc/json-schema/os-list-schema.json),
   worked example in
   [doc/schema-notes.md](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/doc/schema-notes.md))
   listing the image with `"init_format": "systemd"` (or `"cloudinit"`),
   point Imager's Settings → "Custom repository" at it, and the image then
   appears as a normal catalog entry with the *full* customization wizard
   (WiFi/hostname/user/locale) exactly as an end user experiences it. This
   is the path GoSD's own catalog entry uses (`gosd build`'s `os_list.json`
   output, per the locked end-user-flashing decision — see root
   `CLAUDE.md`), and the path the fixtures cited throughout this document
   were captured through.

This is why the flagship flashing path is a custom-repository catalog
entry with `gosd.toml` hand-editing as the always-present fallback, not
"flash the `.img` and click the gear icon" — that GUI step doesn't exist
for a locally-selected file.

## Mechanisms, and when each applies

Imager has exactly three provisioning mechanisms today, selected by a single
`init_format` string carried on the OS-list entry (`"systemd"`,
`"cloudinit"`, `"cloudinit-rpi"`, or empty/`"none"` for "no customization
available"). Valid values are enforced and unknown ones pruned:
[src/oslistmodel.cpp#L24-L31](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/oslistmodel.cpp#L24-L31).
There is **no `custom.toml` mechanism anywhere in the current source** —
`git grep -i "custom.toml"` across the v2.0.10 tree returns nothing. (It may
be conflated in folk knowledge with Alpine's or other distros' unrelated
`usercfg.toml`/cloud-config schemes; it is not part of rpi-imager.)

| `init_format` | Files written to boot (FAT) partition | Generator | Notes |
|---|---|---|---|
| `"systemd"` | `firstrun.sh`, `cmdline.txt` (appended) | `CustomisationGenerator::generateSystemdScript` | The legacy/universal mechanism. Works on any OS that mounts the FAT partition at `/boot` and boots with systemd. |
| `"cloudinit"` | `user-data`, `network-config`, `meta-data`, `cmdline.txt` (appended) | `generateCloudInitUserData` + `generateCloudInitNetworkConfig` | Generic cloud-init (NoCloud datasource). Used for non-RPi distros that already ship cloud-init (e.g. Ubuntu). |
| `"cloudinit-rpi"` | same as `"cloudinit"`, plus an `rpi:` block in `user-data` | same, with `hasCcRpi=true` | Adds the Raspberry-Pi-specific `cc_raspberry_pi` cloud-init module (I2C/SPI/1-Wire/serial/USB-gadget). Gated additionally by the OS entry's `capabilities` list, checked via `imageSupportsCcRpi()` ([src/imagewriter.cpp#L4087-L4090](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/imagewriter.cpp#L4087-L4090)). |
| `""` / `"none"` | nothing | — | Customization step is skipped entirely. This is the effective value for any locally-selected "Use custom" image (see above). |

The dispatch is a single branch in
`ImageWriter::applyCustomisationFromSettings`
([src/imagewriter.cpp#L3873-L3891](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/imagewriter.cpp#L3873-L3891)):
`"systemd"` → `_applySystemdCustomisationFromSettings`; anything else
non-empty → `_applyCloudInitCustomisationFromSettings`. The actual file
writes happen later in `DownloadThread::_customizeImage`
([src/downloadthread.cpp#L2338-L2413](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/downloadthread.cpp#L2338-L2413))
for the normal SD-card/USB write path, or in
`FastbootFlashThread::applyCustomisation`
([src/fastbootflashthread.cpp#L260-L404](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/fastbootflashthread.cpp#L260-L404))
for eMMC-over-USB (fastboot/rpiboot) targets like CM4/CM5 in USB-boot mode —
not relevant to GoSD, which produces plain `.img` files written the first
way.

Every field the `"systemd"` generator emits into `firstrun.sh` is wrapped
unconditionally in both an `imager_custom`-present branch and a manual-
fallback branch (`if [ -f /usr/lib/raspberrypi-sys-mods/imager_custom ];
then ... else ... fi` — see
[src/customization_generator.cpp#L196-L202](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/customization_generator.cpp#L196-L202)
for one example), regardless of what target OS Imager thinks it's writing
for — both branches are always present in the script.

`cmdline.txt` is appended (never fully rewritten unless it didn't exist —
see the Radxa case, below) with, depending on format and content:
- `systemd` only: ` systemd.run=/boot/firstrun.sh systemd.run_success_action=reboot systemd.unit=kernel-command-line.target`
- either cloud-init format, only if cloud content is non-empty: ` ds=nocloud;i=<instance-id>`
- either format, only if a WiFi country was set: ` cfg80211.ieee80211_regdom=<CC>` (this is how country gets applied even when there's no SSID to attach it to in the YAML)

Confirmed on a real capture (Raspberry Pi Imager v2.0.10, custom-repository
catalog flow, macOS, 2026-07-05 — see
`internal/provision/testdata/imager-2.0.10/`): appending, not replacing, is
exactly what happens. A GoSD-built image's own
`console=serial0,115200 quiet init=/init gosd.board=pi-zero-2w
cfg80211.ieee80211_regdom=GB` cmdline survives unchanged, with Imager's
`ds=nocloud;i=rpi-imager-<timestamp>` token appended after it.

### Older behavior (pre-cloud-init, v1.6.2 and earlier)

Before cloud-init support existed, **`firstrun.sh` + `cmdline.txt` editing
was the only mechanism, and it applied unconditionally to any image,
including locally-selected custom images** — there was no `init_format`
concept and no `imageSupportsCustomization()` gate at all (confirmed by
absence: no `_initFormat`/`imageSupportsCustomization`/`customization_supported`
symbol exists anywhere in the v1.6.2 tree). The gear/OptionsPopup was always
visible and always generated `firstrun.sh`, e.g.
[OptionsPopup.qml#L520-L580](https://github.com/raspberrypi/rpi-imager/blob/4a039b78853a87b665b7ab89d819f36af591a1b1/OptionsPopup.qml#L520-L580).
The `init_format`-based gating (and therefore the "Use custom disables
customization" restriction described above) was already present by
v1.7.5, so it predates the current 2.0 rewrite by some margin — this is not
a recent regression, it has applied to every Imager release GoSD is likely
to encounter in the wild.

Cloud-init support for Imager itself was added well before Raspberry Pi OS
adopted cloud-init: rpi-imager could target cloud-init-based distros (like
Ubuntu's preinstalled server images) years earlier. Raspberry Pi OS's own
first-party cloud-init support only shipped in the Debian Trixie release
dated **24 November 2025** — Bookworm and earlier RPi OS releases use only
the `firstrun.sh`/systemd mechanism, and require `init_format: "systemd"`.
(Source: [raspberrypi.com/news/cloud-init-on-raspberry-pi-os](https://www.raspberrypi.com/news/cloud-init-on-raspberry-pi-os/).)
Practically: **almost every Raspberry Pi OS image currently in the wild
(including everything Radxa-adjacent boards would plausibly borrow
conventions from) is on the `firstrun.sh` mechanism** — GoSD's own catalog
entry sidesteps this by declaring `init_format: "cloudinit"` itself.

## WiFi PSK: PBKDF2-hashed, never plaintext, in every format

The PSK is **never written as a plaintext passphrase** to any file Imager
writes. It's derived client-side, in the UI, the moment the user finishes
typing it, via `ImageWriterSingleton.pbkdf2(pwd, ssid)`
([src/wizard/WifiCustomizationStep.qml#L479](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/wizard/WifiCustomizationStep.qml#L479)),
which calls the `Q_INVOKABLE` `ImageWriter::pbkdf2`
([src/imagewriter.h#L339](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/imagewriter.h#L339),
implementation at
[src/imagewriter.cpp#L3997-L3999](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/imagewriter.cpp#L3997-L3999)):

```cpp
QString ImageWriter::pbkdf2(const QByteArray &psk, const QByteArray &ssid) {
    return QPasswordDigestor::deriveKeyPbkdf2(QCryptographicHash::Sha1, psk, ssid, 4096, 32).toHex();
}
```

**Derivation: `PBKDF2-HMAC-SHA1(password, salt=SSID, iterations=4096, dkLen=32 bytes)`, hex-encoded.**
This is the standard WPA2-PSK passphrase→PMK derivation (IEEE 802.11i-2004
Annex H.4) — identical, byte for byte, to `wpa_passphrase` and to what
gosd-init's own `wifiup.DerivePSK` already implements
(`cmd/gosd-init/internal/wifiup/psk.go`, same algorithm, same parameters,
same comment citing the same IEEE annex). No hashing work was needed on the
GoSD side beyond what `wifiup.DerivePSK`/`wifiup.ParsePSKHex` already do —
`internal/provision`'s parser just extracts the 64-hex string (see below).

The only value that ever reaches an artifact is the crypted PSK
(`wifiPasswordCrypt`); the generator prefers it and only falls back to
hashing a legacy plaintext setting itself if the crypted value is missing
([src/customization_generator.cpp#L160-L168](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/customization_generator.cpp#L160-L168)
for firstrun.sh,
[src/customization_generator.cpp#L724-L732](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/customization_generator.cpp#L724-L732)
for network-config) — either way, only the hash is ever serialized. A
literal 64-hex-character input is passed through unhashed (treated as an
already-derived PSK, not double-hashed) — the same "is it 64 hex chars"
heuristic `wifiup.isHexPSK` uses. Open networks (no password) get
`key_mgmt=NONE` (firstrun.sh) or an `auth: {key-management: none}` block
(network-config) — no PSK field at all.

This derivation is unchanged as far back as v1.6.2
([imagewriter.cpp#L970-L984](https://github.com/raspberrypi/rpi-imager/blob/4a039b78853a87b665b7ab89d819f36af591a1b1/imagewriter.cpp#L970-L984)) —
same hash, same iteration count, same key length, just built with OpenSSL's
`PKCS5_PBKDF2_HMAC_SHA1` on non-Darwin platforms instead of Qt's
`QPasswordDigestor` (an implementation detail with no on-disk difference).

Confirmed on a real capture: the WiFi PSK is delivered as a 64-character
hex PBKDF2 digest in `network-config`'s `password` field in every captured
scenario that configured WiFi, never plaintext, matching the derivation
above exactly.

## Field-by-field extraction table

Settings map keys (as consumed by `CustomisationGenerator`,
[src/customization_generator.h](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/customization_generator.h)
and
[src/customization_generator.cpp](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/customization_generator.cpp))
against what actually lands in each file:

| Field | Settings key | `firstrun.sh` (systemd) | `user-data` (cloud-init) | `network-config` (cloud-init) |
|---|---|---|---|---|
| Hostname | `hostname` | `imager_custom set_hostname <host>` (or `echo <host> >/etc/hostname` + `/etc/hosts` sed, if `imager_custom` absent on target) | `hostname: <host>` + `manage_etc_hosts: true` | — |
| WiFi SSID | `wifiSSID` / `wifiSsidOctets` / `wifiSsidOctetsBase64` | `imager_custom set_wlan <ssid> <psk> <country>` (shell-quoted), or a raw `wpa_supplicant.conf` heredoc with `ssid="…"` / `ssid=hex:…` if octets aren't shell-safe UTF-8 | not present in `user-data` | YAML map key under `wifis.wlan0.access-points."<ssid>"` (octets escaped `\xHH` for non-printables) |
| WiFi PSK | `wifiPasswordCrypt` (preferred) / `wifiPassword` (legacy plaintext, hashed before use) | 2nd positional arg to `imager_custom set_wlan`, or `psk=<hex>` line in the wpa_supplicant heredoc | — | `access-points."<ssid>".password: "<hex>"` |
| WiFi hidden | `wifiHidden` / `wifiSSIDHidden` | `-h` flag to `set_wlan`, or `scan_ssid=1` in wpa_supplicant fallback | — | `hidden: true` |
| WiFi country | `recommendedWifiCountry` | 3rd arg to `set_wlan`; also appended to `cmdline.txt` as `cfg80211.ieee80211_regdom=<CC>` | — | `regulatory-domain: "<CC>"` (only when an SSID is present — see cmdline fallback below otherwise) |
| Username | `sshUserName` (defaults to `pi` if unset) | `/usr/lib/userconf-pi/userconf <user> <passwd>` (or manual `usermod`/`chpasswd -e`/sudoers-rename fallback) | `user.name: <user>` | — |
| User password | `sshUserPassword` (pre-crypted: yescrypt or sha256crypt, see below) | 2nd arg to `userconf`, or `chpasswd -e` | `user.passwd: "<hash>"`, `user.lock_passwd: false` | — |
| SSH public key(s) | `sshAuthorizedKeys` / `sshPublicKey` | `imager_custom enable_ssh -k <keys...>`, or heredoc into `$FIRSTUSERHOME/.ssh/authorized_keys` | `user.ssh_authorized_keys: […]` | — |
| SSH enabled (password auth) | `sshEnabled` + `sshPasswordAuth` | `imager_custom enable_ssh`, or `systemctl enable ssh` | `ssh_pwauth: true`, plus `runcmd: [systemctl, enable, --now, ssh]` | — |
| Passwordless sudo | `passwordlessSudo` | writes `/etc/sudoers.d/010_<user>-nopasswd` directly | `user.sudo: ALL=(ALL) NOPASSWD:ALL` **and** a `runcmd` that (re)writes the same sudoers file directly, since `sudo:` isn't reliable on every cloud-init variant | — |
| Keyboard layout | `keyboard` | `imager_custom set_keymap <kbd>`, or writes `/etc/default/keyboard` directly | `keyboard: {model: pc105, layout: "<kbd>"}` | — |
| Timezone | `timezone` | `imager_custom set_timezone <tz>`, or `/etc/timezone` + `dpkg-reconfigure` | `timezone: <tz>` | — |
| I2C/SPI/1-Wire/serial | `enableI2C`/`enableSPI`/`enable1Wire`/`enableSerial` | not generated (systemd format has no interfaces support) | `rpi.interfaces: {i2c, spi, onewire, serial}` — **only** when `init_format` is `cloudinit-rpi` (`hasCcRpi`) | — |
| USB gadget | `enableUsbGadget` | not generated | `rpi.enable_usb_gadget: true` — cloud-init-rpi only | — |
| Raspberry Pi Connect token | `piConnectEnabled` + token | writes token file + enables systemd user units directly in the script | `runcmd` block doing the same via `sh -c` | — |

Auxiliary files always written alongside cloud-init content:
`meta-data` (just `instance-id: rpi-imager-<epoch-ms>`, required so
cloud-init's NoCloud datasource treats the seed as fresh each time it's
regenerated —
[src/downloadthread.cpp#L2378-L2384](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/downloadthread.cpp#L2378-L2384)).

User account password hashing (`ImageWriter::crypt`,
[src/imagewriter.cpp#L3960-L3993](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/imagewriter.cpp#L3960-L3993)):
`yescrypt` for any OS with `release_date >= 2023-01-01`, `sha256crypt`
(`$5$...`) otherwise. This is a **login password hash**, unrelated to the
WiFi PSK derivation — gosd-init has no login shell to apply it to (per the
"no interactive surface" locked decision), so this field is not consumed by
`internal/provision` beyond "don't error out parsing it."

## No `cmdline.txt` on the boot partition (the Radxa case)

This is the direct, source-confirmed answer to "does firstrun injection
break, no-op, or fall back?" for boards like Radxa Zero 3E that don't ship a
`cmdline.txt` at all (U-Boot reads `extlinux.conf`/a boot script, not a
`cmdline.txt` convention):

**It's a silent no-op, not a break.** `DeviceWrapperFatPartition::readFile`
returns an empty `QByteArray` when the file doesn't exist — it logs a
`qDebug()` line and returns cleanly, it does not throw
([src/devicewrapperfatpartition.cpp#L501-L503](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/devicewrapperfatpartition.cpp#L501-L503),
general not-found handling throughout
[L338-L522](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/devicewrapperfatpartition.cpp#L338-L522)).
`_customizeImage` treats that empty read as the starting content and just
appends to it:

```cpp
QByteArray cmdline = fat->readFile("cmdline.txt").trimmed();   // "" if missing
cmdline += _cmdline;                                            // append customization tokens
fat->writeFile("cmdline.txt", cmdline);                         // (re)write
```
([src/downloadthread.cpp#L2406-L2413](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/downloadthread.cpp#L2406-L2413))

So on a Radxa-style image with no pre-existing `cmdline.txt`, Imager
**creates a brand-new one** containing only the customization tokens (a
leading space plus e.g. `systemd.run=/boot/firstrun.sh ...`) and separately
writes `firstrun.sh` to the FAT root. Nothing aborts, nothing else on the
partition is touched or corrupted.

But that new `cmdline.txt` is **never read by anything on a Radxa boot** —
U-Boot builds its own kernel command line from `extlinux.conf`/boot script,
not from a FAT-partition `cmdline.txt` file, so the kernel never receives
`systemd.run=/boot/firstrun.sh` as a boot parameter, so systemd's
`systemd-run-generator` never fires
([systemd-run-generator(8)](https://www.freedesktop.org/software/systemd/man/latest/systemd-run-generator.html):
it only acts on a `systemd.run=` token actually present on `/proc/cmdline`).
The upstream schema documentation says this explicitly as a caveat on
`init_format: "systemd"`:

> "THIS WILL ONLY WORK IF THE FAT PARTITION IS MOUNTED AT /boot in your
> /etc/fstab."
> ([doc/json-schema/os-list-schema.json#L434-L438](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/doc/json-schema/os-list-schema.json#L434-L438))

Radxa images fail that precondition twice over: no `/boot` fstab entry
pointing at the FAT partition in our images' rootfs (there's no rootfs
consuming it at all in gosd-init's minimal design), and no bootloader path
that turns `cmdline.txt` content into `/proc/cmdline` in the first place.
**`firstrun.sh` therefore sits on the boot partition completely inert** —
which is exactly why `internal/provision` only detects its presence and
never parses or executes it: nothing else is ever going to run it for us
either.

The same "missing file = silent no-op, new content gets appended/created"
behavior holds for `config.txt` (only touched `if (!_config.isEmpty())`) and
for the cloud-init files (`user-data`/`network-config`/`meta-data` are
unconditionally created new — there's no pre-existing file to merge with in
the cloud-init case, so "missing file" isn't even a distinct scenario
there).

One asymmetry worth flagging: the **fastboot/USB-boot path**
(`FastbootFlashThread::applyCustomisation`, used for CM4/CM5 eMMC recovery
mode — not GoSD's plain-`.img` path) treats a failed `config.txt`/`cmdline.txt`
read as a hard error and aborts the whole flash
([src/fastbootflashthread.cpp#L304-L332](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/fastbootflashthread.cpp#L304-L332),
[L383-L400](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/fastbootflashthread.cpp#L383-L400)).
GoSD boards aren't flashed through this path, so it doesn't apply to us, but
it's worth knowing the two code paths disagree, in case a future board does
use fastboot.

## What consumes each format on a real Raspberry Pi OS boot

None of this runs on a GoSD image — gosd-init parses cloud-init's files
directly and never executes `firstrun.sh` (see below). This section is
context: what `firstrun.sh`/cloud-init are *designed* to be consumed by on
a stock Raspberry Pi OS boot, which is why the generated content looks the
way it does.

- **`systemd.run=` kernel parameter** → `systemd-run-generator(8)`, a
  built-in systemd generator (not RPi-specific) that turns the parameter
  into a transient `kernel-command-line.service` unit running the given
  command, with `systemd.run_success_action=reboot` causing a reboot on
  success
  ([systemd-run-generator(8)](https://www.freedesktop.org/software/systemd/man/latest/systemd-run-generator.html),
  [kernel-command-line(7)](https://www.freedesktop.org/software/systemd/man/latest/kernel-command-line.html)).
  This is why the mechanism is called "systemd" in `init_format`: it's
  entirely generic systemd machinery, not something Raspberry Pi wrote.
- **`/usr/lib/raspberrypi-sys-mods/imager_custom`** — a helper script
  shipped in Raspberry Pi OS's `raspberrypi-sys-mods` package
  ([RPi-Distro/raspberrypi-sys-mods](https://github.com/RPi-Distro/raspberrypi-sys-mods)).
  `firstrun.sh` calls this if present, falling back to manual file edits
  otherwise (which is what runs on non-RPi-OS targets, and — since it never
  runs at all on GoSD — is irrelevant to us beyond explaining why the field
  table above has two shell branches per field). On current (Bookworm+) RPi
  OS, `set_wlan` writes a NetworkManager keyfile
  (`/etc/NetworkManager/system-connections/preconfigured.nmconnection`),
  **not** `wpa_supplicant.conf` — that fallback format only appears on
  older/non-NetworkManager targets.
- **`/usr/lib/userconf-pi/userconf`** — companion package
  ([RPi-Distro/userconf-pi](https://github.com/RPi-Distro/userconf-pi))
  handling first-user rename/password.
- **cloud-init** (`user-data`/`meta-data`/`network-config`) is consumed by
  cloud-init's own **NoCloud datasource**, which looks for these three files
  on any FAT-labeled/vfat volume; the `ds=nocloud;i=<instance-id>` cmdline
  token lets cloud-init validate its cache without needing a labeled seed
  volume convention. The `rpi:` block in `user-data` is consumed by the
  Raspberry-Pi-specific `cc_raspberry_pi` cloud-init module, only present on
  OS builds that declare the corresponding capability.

## What gosd-init parses, and its precedence

`gosd-init` reads provisioning through `internal/provision`
(`provision.Read`), called once, right after the boot partition
mounts (`cmd/gosd-init/internal/boot/sequence.go`). It consumes exactly two
things, both from cloud-init's YAML:

- **Hostname**, from `user-data`'s top-level `hostname` field
  (`internal/provision/userdata.go`). Every other top-level field —
  `packages`, `apt`, `user`, `ssh_pwauth`, `runcmd`, `keyboard`,
  `timezone`, and anything else the wizard or a hand-written file adds — is
  ignored; a single log line summarizes what was ignored, so nothing is
  dropped without a trace.
- **WiFi networks**, from `network-config`'s
  `network.wifis.<interface>.access-points` map
  (`internal/provision/networkconfig.go`), in file order. gosd-init only
  ever joins one network at a time, so only the first entry is used; the
  rest are kept in the result only so a caller can log what else was
  found. SSID, password, and the `hidden` flag are extracted;
  `regulatory-domain` and every other `network-config` section are not.

`meta-data` is deliberately never read: every captured fixture shows it
containing only cloud-init's own `instance-id` bookkeeping field, nothing
gosd-init consumes.

`firstrun.sh` is detected (an `os.Stat`, nothing more) but never opened,
parsed, or executed — the locked end-user-flashing decision (root
`CLAUDE.md`) puts firstrun.sh parsing out of scope: gosd-init logs one
line pointing the user at `gosd.toml` instead. Executing it was never on
the table anyway, since gosd-init ships no shell. `custom.toml` isn't checked for at all, since it isn't a real
Imager output format (see above) — nothing produces it to parse.

### Precedence

```
gosd.toml  >  cloud-init (user-data / network-config)  >  baked config.json
```

This is implemented, not merely recommended. `boot.Run` reads cloud-init
first and lets a subsequent `gosd.toml` value overwrite the hostname it set
(`cmd/gosd-init/internal/boot/sequence.go`);
`wifiup.ConfigCredentials.Credentials` resolves the same order for WiFi —
`gosd.toml`'s network wins if set, else the first cloud-init network, else
the network baked into `config.json` at build time
(`cmd/gosd-init/internal/wifiup/credentials.go`).

Rationale:

1. **`gosd.toml`** first because it's GoSD's own hand-editable format
   (locked project-wide decision, see root `CLAUDE.md`) — an explicit,
   purpose-built file always wins over an inferred one.
2. **Cloud-init files** next because they're structured YAML, not a shell
   script — parsing YAML is safer and less ambiguous than regexing shell,
   and it's the format GoSD's own catalog entry asks Imager to produce.
3. **Baked `config.json`** last: it's what GoSD wrote at build time, before
   the user ever touched Imager, so any of the above sources represents
   later, more explicit user intent.

`custom.toml` was left out of this chain entirely, rather than kept as a
reserved-but-unused slot, because it has no real producer (see above); and
`firstrun.sh` was scoped out of parsing rather than placed below cloud-init
in precedence, because it's a shell script gosd-init must never execute and
cloud-init already covers the flagship flashing path (see the "no
interactive surface" and "end-user flashing path" locked decisions, root
`CLAUDE.md`).

### WiFi PSK handling

The password extracted from `network-config` is passed through exactly as
found — `internal/provision` never inspects its shape.
`wifiup.ConfigCredentials.Credentials`
(`cmd/gosd-init/internal/wifiup/credentials.go`) is what distinguishes a
64-hex-character value (Raspberry Pi Imager's PBKDF2 output, see above)
from a plaintext passphrase (run through `wifiup.DerivePSK`) — the same
shape heuristic Imager's own generator uses (see above). An empty password
with a non-empty SSID means an open network.

### Fixtures

Every extraction is tested directly against real captures: Raspberry Pi
Imager v2.0.10, captured via the custom-repository catalog flow
(`init_format: "cloudinit"`), macOS, 2026-07-05 — see
`internal/provision/testdata/imager-2.0.10/` and its `capture-notes.md`.
Three scenarios are committed (`wifi-hostname`, `hostname-only`,
`everything`); `internal/provision/provision_test.go` runs the parser
against each and asserts on the extracted hostname, WiFi network(s), and
log output.

`config.txt` and `gosd.toml` are not committed as fixtures: across all
three captures they were byte-identical to GoSD's own builder output
(`pizero2w/templates.RenderConfigTxt`, `gosdtoml.Render`), confirming
Imager left both untouched in the cloud-init flow. One capture-process
quirk to know when reading the raw fixtures: Imager's customization dialog
persists field values between runs, so `hostname: fixture-one` appears in
all three captured `user-data` files rather than a per-scenario name — the
scenario directory name, not the hostname value, is what each fixture
actually tests (see `capture-notes.md`).

## Not yet verified on the bench

Two scenarios remain uncaptured against a real Imager run (tracked on bean
`gosd-qvoq`): an open/no-password WiFi network, and a WiFi SSID containing
non-ASCII or control-byte characters. The latter would confirm the `\xHH`
octet-escaping shown in the extraction table above against real Imager
output — `internal/provision`'s parser tests currently only cover those
shapes with hand-written YAML, not a captured fixture.
