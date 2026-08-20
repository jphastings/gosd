## 0.7.0 (2026-08-20)

### Breaking Changes

#### `gosd build` and `gosd run` refuse a package path that is really a build flag

A package path starting with `-` is no longer passed to the Go toolchain, and
gosd's own `go` invocations now pass Go's `--` terminator before it, so one can
never be read as a build flag again.

This closes a way of getting arbitrary code to run on the machine doing the
build. `gosd build -- -toolexec=/tmp/payload` reached `go build` with
`-toolexec` intact, and `-toolexec` runs a program of the caller's choosing in
place of the compiler — before the app was even compiled, and so with control
over every image that build produced. Reaching it needs influence over gosd's
arguments, which is exactly what a wrapper forwarding a value it does not fully
control gives away: a CI job templating a branch-derived path, a `Makefile`'s
`gosd build $(PKG)`, a script taking a package argument.

An ambient `GOFLAGS` is no longer inherited by gosd's `go` subprocesses either.
It can carry `-toolexec` just as well, and needs no control over gosd's
arguments at all — a `.envrc` picked up on entering a cloned repository, a
modified shell profile, or an inherited CI variable was enough. `GOPROXY`,
`GOPRIVATE` and the other module-fetch variables are still honoured.

Anything that isn't recognisably a package path — a relative path, an absolute
path, or an import path — is now refused with an error naming what was rejected
and what a valid argument looks like. Every documented invocation, `gosd build
.` included, is unaffected.

### Features

#### More actionable build errors, and a new `gadget` sentinel

`gosd build --data-size` now rejects a size that rounds down to less than
one sector (512 bytes) as soon as the flag is parsed, instead of running a
full cross-compile and artifact fetch for every board before failing deep
inside the image writer — the same "fail fast, before any image bytes
exist" contract the 256GiB ceiling check already gave.

A board package's own invariant-violation panic (for example, a u-boot.itb
too big for its locked offset) is now caught and turned into the same
single-line, actionable CLI error every other build failure produces,
instead of reaching the terminal as a raw Go stack trace.

`gadget` now exports `ErrNoController`, wrapped into the error `Apply`
returns when a board has no USB peripheral controller to bind to. This
matches the `errors.Is`-able sentinel convention `sound.ErrNoDevice`,
`emmc.ErrNoEMMC`, and `disk.ErrNoDisk` already give apps that want to
detect a missing device and degrade gracefully instead of failing outright.

#### `disk` and `emmc` no longer adopt a FAT32/exFAT volume on the strength of its label

Formatting a `disk` or `emmc` volume as FAT32 or exFAT wrote the filesystem and
mounted it, with nothing in between forcing those writes to the medium — and
every later boot decided the volume was "already provisioned" by reading its
label back. Neither half is safe. Until a flush happens, an arbitrary subset of
a format's writes may have reached the card, in no particular order; and the
volume label is written near the end of one, so a card that lost power
mid-format could come back with a label that says "ready" over FAT tables that
were never finished. Adopting it handed the app torn cluster chains that
corrupt on first write. The other way round — a label that did *not* land —
left storage that was refused on every boot, forever, despite never having held
anything.

Both are now fixed the way ext4 already worked. A format is followed by a flush
to the medium, and only once that and the mount have succeeded does GoSD write
a reserved, empty `.gosd-established` file into the volume's root. A later boot
adopts the volume only if that marker is there; a volume carrying the app's
label but no marker and no files is crash debris, and is repaired
(reformatted) without needing `destructive`, because nothing was ever written
to it.

**Cards already in the field keep their data.** A FAT32 or exFAT volume
formatted by an earlier release carries no marker, so it is adopted on the
evidence of the files already in it — GoSD's formatters never create a file, so
anything in the root can only have been written by an app that was handed the
mountpoint, which only happens after a format completed. The marker is written
in passing, so the upgrade happens once. The one volume this can reformat is
one with **no files in its root at all**, which by definition has nothing to
lose.

Two smaller behaviour changes come with it, both matching what ext4 has done
since it became the default:

- A FAT32/exFAT volume that matches the app's label and filesystem but cannot
  be mounted is now refused with an error matching both `ErrRefusedFormat` and
  `ErrUnmountable`, rather than a bare mount error — and, with
  `destructive: true`, is reformatted rather than reported.
- `.gosd-established` is reserved. Apps must not delete it; doing so does not
  destroy data, but it costs the volume its proof of a finished format.

Separately, `emmc` can no longer select an eMMC's `boot0`/`boot1`/`rpmb`/`gp0-3`
hardware partitions as a format target. `disk` has excluded them by name since
the day the risk was found; `emmc` merely never encountered one, because the
kernel happens not to label those devices the way its selection looks for. The
exclusion now lives in the code both packages share, so neither can pick one —
formatting an eMMC's boot area leaves a board that no longer boots and cannot
be recovered from its SD card.

#### `--usb-gadget` works on the Radxa Cubie A5E again

Building with `--usb-gadget` for this board now ships a device tree that
disables the USB-C port's host controllers, so the port stays with the
peripheral controller and can present itself as a USB device. Without it the
host side takes the port during boot and the board can never enumerate,
whatever the device tree's `dr_mode` says — which is why the flag has been
refused for this board since that was found on hardware.

The two roles are mutually exclusive on this hardware, which has no circuitry
to detect which is wanted: an image built with `--usb-gadget` cannot use its
USB-C port as a USB host. The USB 3.0 Type-A port is unaffected.

#### `disk` can now wait for a USB drive that is still enumerating

`disk.FormatAndMount` discovers once. That is right for the storage GoSD was
first built around — an NVMe SSD or an onboard eMMC is on an on-SoC bus and is
already enumerated by the time an app's `main` runs — but USB mass storage is
not like that. A stick or an enclosure needs its hub port powered, then a probe,
then a scan, then a medium-ready report: commonly a second or two after the host
controller comes up, and longer through a hub or for a disk that spins up. An app
that reached `FormatAndMount` before all that finished got `ErrNoDisk` for a
drive that was physically plugged in.

The new `disk.Options.Wait` is how long to keep looking:

```go
res := <-disk.FormatAndMountWith("APPDATA", "/storage", disk.Options{
	Wait: 10 * time.Second,
})
```

Its zero value is what shipped before it existed, so no app changes behaviour by
upgrading. There is deliberately no default window: one would stall every app
that treats `ErrNoDisk` as "carry on without a disk", and every board with
nothing attached would pay it on each boot. A long `Wait` is also the honest way
to ask for "use a drive whenever someone plugs one in". `ErrNoDisk` now names the
option when an app never asked to wait, and reports how long it waited when it
did.

#### `gosd cache` inspects and clears the CLI's on-disk caches

`gosd build`/`run`/`build-kernel`/`build-external` already auto-prune their
pinned-download caches to the current pin after every successful run, and the
durable `build-kernel`/`build-external` state directory now keeps only its 8
most recently used entries — everyday growth was already bounded. `gosd
cache` adds manual visibility and control on top of that:

- `gosd cache dir` prints the path of every cache location.
- `gosd cache size` reports how much disk space each one is using, and a
  total.
- `gosd cache clean` deletes the pinned-download caches (board artifacts, the
  CA bundle, ingress binaries, kernel firmware) — always safe, since every
  one is a sha256-verified download the next build/run simply re-fetches.

`gosd cache clean` deliberately leaves the `build-kernel`/`build-external`
state alone by default: each of its entries costs 20-75 minutes of container
build time to reproduce. Pass `--builds` to also clear it.

#### Nothing executes from `/data` any more, and three build inputs are validated like their neighbours

The writable data partition — and any volume the `emmc` or `disk` packages
mount — is now mounted `noexec`, alongside the `nosuid`/`nodev` it already
carried. Nothing a GoSD image ships runs from there (`/app` and every
gosd-shipped helper live in the initramfs rootfs), and `/data` is the one
filesystem on the device an operator can rewrite from a laptop, so the kernel
now refuses outright rather than the property resting on there happening to be
nothing to run.

Three build-time inputs are now checked the way their neighbours already were.
`--publish-base-url` must be an absolute `http(s)` URL with a host, matching
`--support-url` — it is what every download link in a generated `os_list.json`
is built from, and those land in an end user's Raspberry Pi Imager. A
`gosd-kernel.toml` `[[firmware]]` entry's `url` must be `https`, matching every
board manifest gosd ships — a loopback host may still use `http`, since there
is no network path to sit on and that is how a local fixture server is pointed
at. And `sound.Options.Format`'s `Rate` and `Channels`
are rejected when negative, naming the field, instead of arriving at the kernel
as an enormous unsigned value and coming back as a bare `EINVAL`
indistinguishable from hardware that simply cannot do what was asked.

`gadget.Close` no longer reports a gadget as torn down when the teardown
failed. A `Close` that returns an error now leaves the `Gadget` marked applied
— which it is, since the configfs state is still there — so the call can be
retried, and a later `Apply` refuses cleanly instead of walking into the
kernel's `EBUSY`. An `Apply` that fails and then cannot unwind its own
half-written state now says so too, rather than promising a clean slate it
does not have.

#### `gosd-kernel.toml` recipe variants can now share a fragment

A `[kernel.<board-id>]` section's `fragment` key now has a list-typed
sibling, `fragments`: an ordered list of Kconfig fragment paths, merged in
order, each after the last, exactly the way a single `fragment` is already
merged after GoSD's own board fragment. Two recipe variants of the same
board — a cheap one and one that additionally enables DRM, say — can now
list a shared fragment first and their own variant-specific fragment last,
instead of each carrying a full copy of the shared content. `fragment` and
`fragments` are mutually exclusive on the same board. See
[the custom-kernel recipe docs](docs/custom-kernels.md) for the full syntax.

#### The audibility pass no longer goes silent because of one bad control

`Open`'s audibility pass unmutes an ALSA codec that powers up muted — but
until now, one control write that failed (a mixer element this codec
doesn't have, a transient DAPM power race) aborted the whole pass, silently
skipping every later, unrelated unmute in the list. A board could end up
quieter than it should be for a reason that had nothing to do with the
control that actually failed.

The pass now attempts every change regardless of earlier failures, and
reports every failure together. `Device.Mixer().Changed` still lists what
succeeded; a caller that wants to know what didn't can check the error
`OpenWith` — or `Device`'s own `Mixer` — returns.

##### `SetControl` can now address a control at any index, with an actionable not-found error

`Device.SetControl` only ever matched `Control.Index == 0`, so a card with
more than one control sharing a name (real hardware — see `Control.Index`)
could only ever have the first one addressed, and the error read "no
control named %q" even when the name existed at a different index.

`Device`s from `Open`/`OpenWith` now also implement the new
`sound.IndexedControl` interface:

```go
if ic, ok := dev.(sound.IndexedControl); ok {
	err := ic.SetControlIndexed("Some Mixer Switch", 1, 1)
}
```

and the not-found error now says which index a matching name was actually
found at, when there is one, instead of reading like a typo'd name.

### Fixes

#### The artifact cache can no longer be used to plant a backdoor

`gosd build` downloads each board's kernel and bootloader from a GoSD
artifact release and verifies every file against the digests in that
release's `manifest.json`. The manifest itself was the exception: once
cached, it was trusted because it was there, and it was the only thing
vouching for the files beside it.

That made the cache directory a place to install a backdoor. Anything running
as you — a compromised editor extension, an npm or pip postinstall — could
drop a modified kernel into `~/.cache/gosd/` next to a `manifest.json` listing
that kernel's digest. The pair agreed with each other, so every later build
took the offline cache-hit path, made no request that might have noticed, and
baked the result into every image you flashed, outliving removal of whatever
planted it.

The digest of the pinned release's `manifest.json` is now compiled into
`gosd` and re-checked every time the manifest is read, from the network and
from the cache alike — the same way the CA bundle and every third-party blob
have always been pinned. A tampered cache now costs a re-download rather than
a compromised image, and offline it fails loudly instead of quietly using it.

Builds that supply their own artifacts with `--artifacts-dir` are unaffected.
Two smaller hardening fixes ride along: a board tarball that keeps
decompressing is abandoned rather than allowed to fill the disk, and downloads
now give up on an upstream that accepts the connection and then goes silent,
instead of hanging forever.

#### Board images are now built from artifacts v0.10.2

`gosd build` downloads the board kernels and bootloaders published as
v0.10.2, up from v0.10.0, which brings:

- Cubie A5E images now boot the 1GB RAM variant
- The Cubie A5E kernel build now produces a USB-gadget variant device tree
- Cubie A5E U-Boot no longer scans USB on every boot

#### One oversized file on the boot partition can no longer stop a device booting

Everything on a device's boot partition is editable by anyone who can plug
the card into a computer, and `gosd-init` runs as PID 1 with its entire root
filesystem in RAM — where Linux panics rather than killing init to reclaim
memory, and the file that caused it is still there on the next boot. Two
inputs it read without a ceiling now have one:

- **A cloud-init seed is size-capped.** A `user-data` or `network-config`
  file larger than 256 KiB — three orders of magnitude past what Raspberry
  Pi Imager writes — is ignored with a line naming it, rather than read and
  parsed into roughly forty times its own size in memory. A seed that isn't
  an ordinary file is refused before it's opened, so a named pipe left in
  its place can't stall a boot indefinitely.

- **The `config/` tree is bounded as a whole, not just per file.**
  Individual settings were already capped at 64 KiB each, which a card full
  of small ones walks straight past; now the tree has a ceiling too (1 MiB,
  room for around four thousand settings), as does how deeply it will be
  walked. Reaching either logs one line, and every setting not read keeps
  the value the image was built with.

Crash reports also got stricter about their own redaction labels. The
`{$VAR_NAME}` and `{secret: ...}` placeholders that stand in for removed
values are built from names gosd doesn't choose — a file name on the card,
or the label your app passes to `fault.RegisterSecretString` — and are now
guaranteed to be single-line labels of a sensible length, so neither can
reshape the report it appears in. One that can't be used as a label is
replaced with `{redacted}` outright rather than trimmed to a fragment that
still reads like a name; the value it stands for is removed either way.

#### Crash reports redact what your app supplied, and leave gosd's own words alone

Three corrections to what `LAST_FATAL_ERROR.md` scrubs.

The **error code** is now redacted like every other field. It is your text —
an app can build one from an upstream failure, a request id or an account
identifier — and it was being written into the report's header untouched
because the header was assumed to hold only values gosd generates itself.

**gosd's own wording is no longer rewritten.** Redaction used to run over the
finished document, boilerplate included, so an ordinary environment value
that happened to be a word in gosd's prose rewrote the sentences your user
reads: an app with `APPNAME=weatherbox` got `# {$APPNAME} crash report`, and
any value of `computer` mangled the line explaining what the file is. Those
strings are compiled into gosd and can never contain your secret, so they are
left exactly as written. Everything the report carries in from your side —
the error code, all four written sections, the technical detail, and the app
name, version and support URL baked into the image — is still scrubbed
wherever it lands.

**Tunnel credentials are now redacted too.** A Cloudflare tunnel token
becomes `{ingress: cloudflared-token}` and a Tailscale auth key
`{ingress: tailscale-funnel-authkey}`, so the secrets gosd-init holds for
itself are covered by the same net as the ones it holds for you.

#### A misbehaving DHCP server can no longer dictate how often a device renews its lease

The renewal schedule for a DHCP lease is built from three numbers the server
sends — T1, T2 and the lease time — and `gosd-init` used to act on them as
sent. A server offering a lease time of zero therefore scheduled the next
renewal in the past, and the device renewed as fast as the network would let
it, reassigning its address, replacing its default route and rewriting
`/etc/resolv.conf` on every pass. That is a lot of load for a single-core
board to carry indefinitely, and nothing on the device can be told to stop:
there is no shell to log into.

Lease timers are now bounded before anything is scheduled from them. Renewal
never happens more than once a minute (the floor RFC 2131 already uses for a
client's own retransmissions), a missing or zero lease time falls back to an
hour rather than to "immediately", and a lease longer than a day — including
the "infinite" lease, whose arithmetic used to overflow into a renewal
permanently in the past — is re-confirmed daily instead of trusted forever.
When timers have to be corrected, the console says so once, naming what the
server offered, so a rogue or simply broken server on the network is
visible rather than silent.

Ordinary leases are unaffected: their timers are already inside these
bounds, and are used exactly as the server sent them.

#### `gosd build-kernel` now refuses a fuzzy or skipped device-tree patch instead of applying it silently

Every `.patch` file `gosd build-kernel` applies — GoSD's own board patches and
a developer overlay's `patches` — now applies with `patch -p1 --fuzz=0`
instead of `--forward`. A hunk against a freshly cloned, exactly-pinned
kernel source tree can never legitimately need fuzzy context matching or be
"already applied"; if either happens, something (most likely a kernel-tag
bump shifting nearby source lines) has silently changed what the patch
actually does. The build now fails loudly, naming the offending patch,
instead of shipping a kernel that silently missed the peripheral enablement
the patch was meant to provide. Write overlay patches against the pinned
kernel tag your board's `internal/kernelspec` entry uses, not a nearby one.

#### A setting restored after a reflash can no longer do more than one you typed

`gosd-init` keeps your settings on the data partition so a reflash puts them
back. That copy has never been authenticated — the data partition is the one
thing a reflash leaves alone, and anything able to write there could leave a
setting behind for a freshly flashed card to pick up. Restoring one was also
skipping checks the same value goes through when you type it onto the card
yourself. Three ways it could:

- **A restored hostname could forge an `/etc/hosts` entry.** One carrying a
  newline added an attacker-chosen name-to-address mapping, which Go's
  resolver consults ahead of DNS for every lookup your app makes — so the
  app's API endpoint could be silently re-pointed on a device its owner had
  just reflashed. `/etc/hosts` is now rendered with no hostname line at all
  for a name that isn't one, whatever its caller believes.
- **An app environment variable's name is now checked at runtime**, to the
  same rule `gosd build` enforces, rather than only at build time.
- **A NUL byte in any value is refused**, on the card and in the kept copy
  alike. One in an app environment variable makes `execve(2)` fail, so a
  single stray NUL stopped `/app` starting on every boot — and went on doing
  so through the reflash performed to fix it.

**A restore now says so on the console**, naming the partition it came from
and how many settings it put back.

Every setting is still restored, credentials included: putting back what you
put on the card is what the kept copy is for. The trade-off is now written
down in the config tree's guide — **a reflash is not a factory reset**, and
clearing the data partition is the operation that resets a device.

#### `examples/usbwebsite` no longer publishes the device's own secrets

The example shared the SD card's data partition two ways that both handed out
more than the website. It served `/data` itself over HTTP — and `http.FileServer`
has no notion of a hidden file, so `http://<board>.local/.gosd/config/values/wifi/passphrase`
returned the WiFi passphrase to anyone on the network. It also offered that
same partition as a read-write USB drive, which gave any computer the cable
reached the passphrase, any ingress token and the Tailscale node's private
key, plus write access to the `/data/.gosd` area that survives a re-flash.

Both halves are now scoped to a directory the app owns. On the SD-card path
the site lives in a `website` folder and only that folder is served, and the
partition is no longer offered over USB unless the operator writes `yes` into
`config/env/WEBSITE_SHARE_DATA` — the app logs what that exposes when they do.
Boards with onboard eMMC are unchanged: that volume is the app's alone, so it
is still served from its root and shared freely.

If you run this example on an eMMC-less board, note that the USB drive no
longer appears by default, and that the site's files now belong in the
`website` folder of the data partition rather than at its root.

`gadget.MassStorage`'s documentation now says outright that a LUN is the whole
volume — there is no sharing a subdirectory — and that `ReadOnly`'s zero value
hands an unauthenticated host write access to all of it.

## 0.6.5 (2026-08-18)

### Features

#### The status LED's running and fatal states have changed

v0.6.4 introduced the status LED with a fast blink for a recorded fatal
error. On real hardware that signal does not exist: `gosd-init` halts the
board immediately after recording the error, and a halted kernel stops
driving the LED, so the fast blink lasted about a tenth of a second and then
went dark. The release notes claimed it kept blinking through the halt. It
did not.

The three states are now:

| State | LED | Previously |
| --- | --- | --- |
| Booting | Flashes evenly, 250ms on/off | unchanged |
| Running | Blips briefly, 50ms on / 950ms off | solid on |
| Fatal | Solid on | fast 125ms blink |

Fatal is steady because a steady level is the only thing that can outlive the
halt. Running became a blip so that it stays clearly distinct from a solid
LED, and so a healthy board reads as alive rather than merely lit.

If you were relying on the previous meanings — solid for healthy, fast blink
for broken — those two have effectively swapped, and a device that has
halted is now the one showing a steady light.

One caveat worth knowing: for the fatal state to survive the halt at all, a
board's device tree has to mark the LED as retaining its state through
shutdown. No board ships that yet, so for now the LED still goes dark once
the device halts. Everything before the halt is unaffected, and the
[status LED guide](docs/status-led.md) tracks where this stands.

## 0.6.4 (2026-08-17)

### Features

#### Boot state shows on your board's onboard status LED

A GoSD device is headless, and until now there was no way to tell "still
booting" from "wedged" from "running fine" without a serial cable. Every
supported board has at least one software-controllable LED, and `gosd-init`
now uses it as that signal automatically — no code changes, no config:

- Blinks slowly (250ms on/off) while booting.
- Blinks fast (125ms on/off) if a [fatal error was recorded](docs/crash-reports.md).
- Solid on once your app has started and been handed control.

`gosd-init` picks the LED marked ACT, or the activity/status LED, or the
green LED, or whichever LED the board has, following its own device tree —
the [full selection rule and per-board table](docs/status-led.md) covers
which LED that is on each board. The blink itself is driven by the kernel's
own `timer` trigger, not a goroutine, so it keeps blinking through a fatal
halt or a wedged `gosd-init`.

## 0.6.3 (2026-08-17)

### Features

#### `gosd version` says which board artifacts your images will be built from

`gosd` had no way to report its own version, and no way at all to answer the
question that decides whether an image boots: which release of board kernels
and bootloaders it downloads.

```console
$ gosd version
gosd:      v0.6.2
artifacts: v0.10.2
go:        go1.26.5
```

`gosd --version` prints the same. A binary built from a checkout reports its
commit and whether the tree was modified, so "it works on my machine" is
answerable. When a board boots with one `gosd` and not another, the artifacts
line is usually where they differ.

### Fixes

#### Board images are now built from artifacts v0.10.2

`gosd build` downloads the board kernels and bootloaders published as
v0.10.2, up from v0.10.0, which brings:

- Cubie A5E images now boot the 1GB RAM variant
- The Cubie A5E kernel build now produces a USB-gadget variant device tree
- Cubie A5E U-Boot no longer scans USB on every boot

## 0.6.2 (2026-08-17)

### Fixes

#### `--usb-gadget` now refuses for the Radxa Cubie A5E instead of building an image that can't work

Hardware testing showed the Cubie A5E cannot present itself as a USB device
at the currently pinned board artifacts: the USB-C port's host controllers
share a phy with the peripheral controller, and with nothing on the board to
arbitrate between them the host side wins every boot, so the port never
enumerates. The board's device tree pins peripheral mode, which is what
GoSD's earlier support claim was based on, but that is not enough on its own.

Building with `--usb-gadget` for this board now fails with an explanation
rather than producing an image that looks correct and cannot work. Support
returns once a board artifacts release carries the variant device tree that
disables those host controllers — at which point USB-C host mode becomes the
trade-off, since an image can serve one role or the other but not both. The
USB 3.0 Type-A port is unaffected.

#### Boards with no hardware entropy source now get a DHCP lease reliably

On a board whose kernel has no random-number source, the DHCP client could
fail to build its first packet at all — it drew the transaction ID from the
kernel's cryptographic pool, which stays unavailable for the first several
seconds of boot on such hardware. The board came up, started the app, and
silently never joined the network. Transaction IDs no longer depend on that
pool.

Separately, a board that cannot get an address now keeps reporting it on the
console at a backing-off interval, instead of logging one failure and going
quiet — so an unreachable board says why.

#### A data partition reformatted from ext4 to FAT32 no longer halts the device

Formatting a volume as FAT32 over a previous ext4 volume left the old ext4
superblock intact, because the FAT32 writer never touches the offset it sits
at. gosd-init then identified the dead filesystem in preference to the live
one and halted the board on its next boot, reporting corruption and the old
volume's label. Establishing a volume now clears any previous filesystem's
signatures first, so changing `--data-filesystem` between releases reformats
cleanly — as documented — rather than stopping a healthy device.

## 0.6.1 (2026-08-14)

### Features

#### Releases are now prepared by change files and a release PR

Each user-facing change ships a small markdown change file; a bot-maintained release PR accumulates them and, when merged, tags and publishes the CLI, artifacts, and npm releases with real release notes.
