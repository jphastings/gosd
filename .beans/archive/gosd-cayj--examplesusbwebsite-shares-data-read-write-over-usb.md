---
# gosd-cayj
title: examples/usbwebsite shares /data read-write over USB, exposing the WiFi PSK and ingress tokens to any host
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:15:09Z
updated_at: 2026-08-20T06:56:27Z
---

**Severity: High.** GoSD's own reference example hands plaintext credentials
and write access to anyone who plugs in a USB cable.

## Verified

- `examples/usbwebsite/main.go:337` —
  `gadget.MassStorage{Path: st.device, Removable: true}`. **`ReadOnly` is
  not set.**
- `gadget/massstorage.go:27-28` — `ReadOnly bool`, zero value `false`. The
  package's own `TestMassStorageFlagsDefaultOff` confirms the default is
  off. So an omitted field means read-write, silently.
- `st.device` is the **data partition** (`main.go:242-256`).
- `cmd/gosd-init/internal/provsnapshot/provsnapshot.go:648` renders the
  effective gosd.toml — WiFi passphrase and any Cloudflare/Tailscale token
  included — onto `/data` on every boot. The package's own comment
  (`:645-646`) acknowledges it holds the tunnel token "the same trust level
  as the WiFi passphrase already stored here in plain text."

The example targets eMMC-less boards (the Pi Zero W family) explicitly, so
this is the path those boards take, not an exotic configuration.

## Attack

Plug the running device into any computer. It enumerates as a removable
mass-storage device. Mount it — FAT has no hidden-file concept, and a macOS
or Windows host shows `.gosd/` plainly. Read
`.gosd/provision-snapshot/gosd.toml`: the WiFi PSK for the network the
device is on, and any tunnel token, in full.

Then write to it. Because the share is read-write, the same USB cable lets
an attacker plant a provisioning snapshot — which, being unauthenticated
(sibling bean), survives the owner's reflash and re-provisions the device.
The two findings chain into persistence that outlives the owner's cleanup.

No case to open, no card to remove, no credentials required.

## Fix

1. `examples/usbwebsite`: set `ReadOnly: true` for the eMMC-less data-partition
   share, or point the share at a dedicated subdirectory image rather than
   the partition that holds the snapshot.
2. Consider inverting the `gadget.MassStorage` default so write access is
   opt-in (`Writable bool`) rather than opt-out. A field whose zero value
   grants an unauthenticated host write access to a block device is the
   wrong default for this library, and the example proves the default gets
   taken by accident.
3. Document the interaction in `docs/runtime.md` beside `gadget.MassStorage`:
   sharing the data partition shares the provisioning snapshot's secrets.
4. Add an explicit refusal in `gadget/massstorage.go` for the disk backing
   `/` or `/boot`. Today the only protection is `mountedAt`/`isPartitionOf`
   (`:91-150`), which blocks a **currently mounted** path — it is incidental
   that gosd-init keeps /boot mounted, not a rule the package enforces. If
   that ever changes, `MassStorage{Path: "/dev/mmcblk0"}` exposes the kernel
   and gosd.toml read-write, i.e. code execution on next boot.

## Todos

- [x] `ReadOnly: true` (or a non-secret-bearing backing store) in examples/usbwebsite — resolved as *don't share the volume at all* without consent; see "Shape chosen"
- [x] Decide on inverting the MassStorage write default; if kept, document the zero value loudly on the field — **kept**, documented loudly; reasoning in "Decisions"
- [x] Refuse the boot/root disk explicitly in gadget/massstorage.go — **deferred to `gosd-ix0r`**; not implementable from the signals available today, see "Deferred"
- [x] docs/runtime.md: call out that sharing /data shares the snapshot's plaintext secrets


## Shape chosen (2026-08-20)

**A LUN is the whole volume, so an app may only share a volume it owns
outright — and it may only *serve* a directory it owns.** Both halves of
usbwebsite publish files, and both are now scoped to a directory the app
owns rather than to whatever the volume happens to hold.

- **eMMC path: unchanged.** The app formats that volume for the website and
  nothing else on the device writes to it, so its root is the site and it is
  shared read-write with no ceremony. This is the shape the example teaches;
  it is safe *by construction*, not by a flag.
- **SD data partition, serving:** the site now lives in `/data/website` and
  only that folder is served.
- **SD data partition, USB:** not shared by default. Sharing requires
  `config/env/WEBSITE_SHARE_DATA=yes`, mirroring the example's existing
  `WEBSITE_WIPE_EMMC` consent knob; the app logs exactly what a drive would
  expose, both when declining and when consenting.

### A second, worse leak found while verifying — unauthenticated and *remote*

The bean's finding is real but understates the exposure. The example also did
`http.FileServer(http.Dir("/data"))`, and `http.Dir` applies **no** dotfile
filtering (`net/http/fs.go`'s `Dir.Open` cleans the path and opens it —
nothing else). Verified empirically against a fixture tree:

    GET /.gosd/config/values/wifi/passphrase  ->  200 "hunter2"

So the WiFi passphrase was readable over HTTP by anyone who could reach port
80 — no cable, no physical access at all. That is a wider channel than the
USB one and is what makes the "serve a subdirectory" half of the fix
unconditional (no consent knob).

### What is actually on `/data` today

The bean cites `cmd/gosd-init/internal/provsnapshot`, which no longer exists
— epic `gosd-rw6n` replaced it with the config tree + `configstore`. The
finding survives the refactor at new paths:

- `/data/.gosd/config/values/…` — `configstore`, which by design records
  every setting differing from the shipped image. `wifi/passphrase`,
  `ingress/cloudflared/token` and `ingress/tailscale-funnel/authkey` are all
  config-tree values, so an operator-provisioned board keeps them there in
  plain text.
- `/data/.gosd/tailscale/` — `tsfunnel.StateDir`, i.e. the tailnet node's
  private key.

## Decisions

**Keep `MassStorage.ReadOnly`; do not invert to `Writable`.** Three reasons:
(1) `ReadOnly` maps one-for-one onto the kernel's `lun.0/ro` attribute, which
is what keeps the package's configfs mapping auditable; (2) inverting would
silently reverse the meaning of every existing zero-valued literal on
upgrade, and there is no way to make that fail loudly; (3) most importantly,
**it would not have prevented this bug** — the disclosure here is a *read*,
and a read-only LUN still hands the host every byte of the volume. Inverting
would buy false comfort for the smaller half of the problem. The field's
doc comment now says outright that the zero value grants an unauthenticated
host write access to the whole backing store, and that `ReadOnly` is not a
confidentiality control; the example writes `ReadOnly: false` explicitly with
a comment, so nobody copies an omitted field by accident.

**Consent gate rather than a hard refusal, on the SD path.** A hard refusal
would have reversed `gosd-4ajn`'s locked intent (JP, 2026-07-25: "use space
on the SD card for the USB-gadget mountable data volume — that way we can
test with the rpi zero and rpi zero 2w"), which exists precisely so the
eMMC-less Pi Zeros can exercise the mass-storage gadget path — and no board
has completed that bring-up yet. The gate keeps that vehicle available while
making the default safe: **a Pi Zero bench test of the drive path now needs
`WEBSITE_SHARE_DATA=yes` on the card**, which is called out in the README's
power-topology note and printed by the app itself when it declines.

### Alternatives rejected

- **`ReadOnly: true` on the data-partition share** (the bean's first
  suggestion): stops the write half only. The headline finding is reading the
  PSK, which a read-only LUN still permits. Insufficient on its own.
- **A dedicated FAT image file on `/data` as the LUN backing** (the bean's
  second suggestion): still the right idea in the abstract, and still blocked
  by exactly what `gosd-4ajn` recorded when it rejected it. Serving from it
  needs a loop device, and `CONFIG_BLK_DEV_LOOP` is asserted by **no** board
  fragment — the only boards whose committed `kernel.config` shows it are the
  three Pis, and per CLAUDE.md those snapshots are not capability claims.
  Creating it needs a FAT32 formatter an example cannot reach (`internal/…`
  is off-limits, and the go-diskfs depguard fence in `.golangci.yml` allows
  exactly three importers, none of them an example). Recorded here so the
  next person doesn't re-derive it.
- **Serving a subdirectory but still sharing the whole partition over USB:**
  fixes the remote leak, leaves the physical one. Taken as *half* the fix,
  not the whole of it.

## Deferred

**"Refuse the boot/root disk explicitly in `gadget/massstorage.go`" is not
implementable from the signals available today**, so it is left unchecked for
a follow-up bean rather than faked. The package can only learn which device
is the boot disk from the mount table — which is what `mountedAt` already
consults — so an "explicit" refusal would be the same check with a different
name. GoSD boots from an initramfs, so `/proc/cmdline`'s `root=` names no
block device, and nothing else in the kernel marks the boot medium
portably. A real fix needs `gosd-init` to *publish* the disk it booted from
(e.g. a file under `/run/gosd/`) for the `gadget` package to refuse against;
that is a gosd-init feature, not an example fix. The incidental protection
noted in the bean does hold in practice today — gosd-init keeps `/boot`
mounted for the life of the device — but it remains incidental.


## Summary of Changes

PR #342.

- **`examples/usbwebsite/main.go`** — `storage` now carries `contentDir` (the
  only directory served) and `shareDevice` (set only for a volume that may be
  handed to a USB host), replacing the old `mountpoint`/`device` pair where
  both roles were the same path.
  - eMMC path: `contentDir` is the mount root and `shareDevice` is the block
    device, exactly as before — the app owns that volume outright.
  - Data-partition path: `contentDir` is `/data/website`, and `shareDevice` is
    empty unless `WEBSITE_SHARE_DATA` is set, in which case the app first logs
    what the drive exposes. The decision is a small pure function,
    `dataStorage(device, consented)`, so it is unit-testable.
  - New `prepareContent` creates the content directory and seeds the starter
    page *before* the share/serve decision, so a shared drive already shows
    the folder the files belong in.
  - The `gadget.MassStorage` literal writes `ReadOnly: false` explicitly, with
    a comment on why that is right only for a volume the app owns.
  - Starter page and package doc rewritten for the two routes.
- **`examples/usbwebsite/main_test.go`** — `TestDataStorage` pins the three
  security-relevant behaviours (never serves the mount root; no drive without
  consent, with an explanation and no mount closures; drive plus closures once
  consented) and `TestPrepareContent` covers the fresh-card case.
- **`examples/usbwebsite/README.md`** — new "Only publish a volume that is
  yours alone" section explaining *why*: the eMMC/data-partition ownership
  difference, `http.FileServer`'s lack of dotfile filtering, and why a LUN
  cannot be scoped to a subdirectory. This is the deliverable a developer
  copying the example needs.
- **`gadget/massstorage.go`** — "A LUN is the whole volume" section on the
  type, and a loud note on `ReadOnly` that its zero value grants an
  unauthenticated host write access and that it is not a confidentiality
  control. No behaviour change.
- **`docs/runtime.md`** — two new bullets in the USB gadget section: only
  share a volume you own, and do not share (or serve the root of) the data
  partition, naming what is on it.
- **`.changeset/usbwebsite-data-partition-disclosure.md`** — `gosd: patch`,
  written as a release note including the behaviour change eMMC-less users
  will notice.
- **Follow-up filed:** `gosd-ix0r` for the library-level boot-disk refusal.

**Not changed, deliberately:** `gadget.MassStorage.ReadOnly` keeps its name and
polarity (see "Decisions"), and the data partition remains *shareable on
request* rather than refused outright, so `gosd-4ajn`'s Pi Zero bring-up
vehicle survives.

**`gosd-cgpr` (usbwebsite restart loop) is unaffected** — neither fixed nor
worsened. Its fix (idle instead of exit on the paths that need outside action)
lives in `main`/`claimStorage` and is untouched; every path added here either
idles or serves, and nothing new exits.

**Verified:** `go test ./...`, `go vet ./...`, `gofmt -l .` (clean),
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`, plus
`GOOS=linux GOARCH=arm64` and `GOOS=linux GOARCH=arm GOARM=6` builds of the
example. All 15 CI checks green on #342.
