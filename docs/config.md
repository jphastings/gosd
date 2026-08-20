# The config tree: every setting on the card

Every image `gosd build` produces carries a `config/` directory at the root
of its boot (FAT) partition. Inside it, every setting the device understands
— its hostname, the WiFi network to join, the app's own environment
variables, an internet tunnel's token — is its own plain text file: one
setting, one file, one value. Plug the card into any computer, open the file
you want to change in a plain-text editor, type the new value, save, and put
the card back. `gosd-init` reads the whole tree once, early in boot, after
mounting the boot partition.

It is written for a non-technical audience. Every value file sits beside a
`<name>.explain.md` that explains, in plain language, what that setting does
and what to type into it — that sidecar is the only documentation whoever is
holding the card will ever see, so `gosd build` refuses to ship a setting
without one. A malformed or surprising value is logged to the serial console
and skipped, never fatal.

This page is for **app developers**: the tree's layout, how to add and
document your own settings at build time, the rules a value's file name has
to follow, and how a setting behaves across an upgrade. For the full
runtime contract — precedence against a Raspberry Pi Imager wizard, what
`gosd-init` does with each setting once it's read — see
[the runtime contract](runtime.md). For the internet-tunnel settings
specifically, see [the ingress guide](ingress.md). For splicing a setting
into a downloaded image without a rebuild, see
[image injection](image-injection.md#injecting-settings).

## The tree layout

A full-featured build's `config/` directory looks like this:

```
config/
├── explain.md
├── hostname
├── hostname.explain.md
├── data_flush
├── data_flush.explain.md
├── wifi/
│   ├── explain.md
│   ├── ssid
│   ├── ssid.explain.md
│   ├── passphrase
│   └── passphrase.explain.md
├── env/
│   ├── explain.md
│   └── <your app's own settings, one file each>
└── ingress/
    ├── explain.md
    ├── cloudflared/
    │   ├── explain.md
    │   ├── token
    │   ├── token.explain.md
    │   ├── hostname
    │   ├── hostname.explain.md
    │   ├── port
    │   └── port.explain.md
    └── tailscale-funnel/
        ├── explain.md
        ├── authkey
        ├── authkey.explain.md
        ├── hostname
        ├── hostname.explain.md
        ├── port
        ├── port.explain.md
        ├── funnel_port
        └── funnel_port.explain.md
```

Every value file is read the same way: its bytes, trimmed of leading and
trailing newlines. An empty file — including one that holds nothing but
padding, see "Padding is the reservation" below — means that setting is
**unset**. For a setting that's either on or off (`data_flush`), any
non-empty content means "enabled"; there's no particular word to type.

An `ingress/<agent>/` directory only exists in the tree at all when that
agent's binary is actually baked into the image (`gosd build --ingress
<agent>`) **and** the board's architecture supports it — Cloudflare Tunnel is
arm64-only, so a `pi-zero-w` image never carries `config/ingress/cloudflared/`
even empty. A card never documents a tunnel the device it came from has no
way to run.

## Editing a value on a mounted card

Open the file you want to change in any plain-text editor — Notepad on
Windows, TextEdit on a Mac, `nano`/`vim` on Linux — type the new value, and
save.

**On a Mac, watch out for TextEdit's rich-text default.** TextEdit opens
plain files in rich-text mode unless you tell it otherwise, and saving in
that mode can corrupt the file. Before saving, use **Format → Make Plain
Text** (every value file's own `.explain.md` repeats this warning, since it's
the mistake most likely to actually happen).

Whatever you save takes effect the next time the device starts — restart it,
or power it on for the first time, to apply a change.

## `explain.md` files

Every value file has a matching `<name>.explain.md` beside it — `wifi/ssid`
is documented by `wifi/ssid.explain.md` — and every directory that groups
settings has its own `explain.md` describing what the group is for. These
are documentation only: editing or deleting one changes nothing about how
the device behaves. Read them before you change the value they describe.

At **build time**, `gosd build` refuses to ship a value with no explanation
at all — its own, or one it inherits from gosd's built-in defaults when your
`--config-dir` overrides the value but not the prose (see "Your own
settings" below). A setting nobody can explain is worse than no setting, so
this is a hard build failure, not a warning. At **runtime**, `gosd-init`
never requires an `.explain.md` to be present — a card missing one (someone
deleted it, or a very old card predates a setting your rebuilt image added)
just means one setting nobody documents to that particular reader; it never
stops the device from booting.

## Your own settings (`gosd build --config-dir`)

gosd ships its own tree — the `hostname`, `wifi/`, `data_flush`, `env/` and
`ingress/*` layout shown above — as a real directory baked into the `gosd`
binary itself. Your app overlays its own settings onto that tree, file by
file, with `gosd build --config-dir <dir>`:

```sh
gosd build . --board pi-zero-2w --config-dir ./config
```

Every file your directory provides wins over gosd's own file at the same
path — override `hostname.explain.md` to change how that setting is
described, or add a brand-new file at `env/API_TOKEN` to add a setting gosd
never shipped. A file you *don't* provide is inherited unchanged from gosd's
defaults, explanation included, so overriding `env/API_TOKEN` without also
writing `env/API_TOKEN.explain.md` is fine — as long as you wrote the
sidecar for a genuinely new setting.

With no `--config-dir` flag at all, `gosd build` looks for a `config/`
directory sitting beside your app's main package and uses it automatically
if it exists; an explicit `--config-dir` that doesn't exist is refused
outright — a typo'd path silently falling back to gosd's bare defaults would
ship an image missing every setting your app actually needs.

Your own settings usually live under `env/<NAME>` (an app environment
variable — see [the runtime contract's environment-variable
section](runtime.md#app-environment-variables) for how those reach `/app`),
but nothing stops you shipping a whole new top-level file or directory for a
device-level setting your app reads directly off the card.

## Reserved names and the FAT-junk rules

A value's name may contain letters, digits, periods, hyphens and underscores
— periods are fine (a setting genuinely called
`google-service-account.json` is a legal name), but nothing that would need
escaping on the way to a card, a manifest, or a terminal. Beyond that shape,
`gosd build` refuses several kinds of name outright, because the device
either can't tell them from a real setting or has already claimed them for
itself:

- **`explain.md` and any `*.explain.md` name.** Those are documentation
  slots, not settings.
- **Any name ending in `.new` or `.unused`.** The device writes files with
  those suffixes onto the card itself — see "Keeping your settings across a
  reflash" below — so a setting can never be named that way.
- **Any name starting with a period**, including the ones a computer writes
  without asking: macOS's `.DS_Store` is the one you will actually hit if
  you ever browse a `--config-dir` folder in Finder before building. The
  device ignores dot-files on the card, so a setting named this way would
  silently never take effect — `gosd build` catches it at build time
  instead, naming the file to delete.
- **A macOS AppleDouble file** (a `._` -prefixed companion Finder or an
  external drive can write for every file it touches on a FAT-formatted
  volume).
- **`Thumbs.db` and `desktop.ini`** — Windows Explorer's own metadata files.
- **Two names that differ only in capitalization.** FAT is
  case-insensitive, so `Env/api_token` and `env/API_TOKEN` can't coexist on
  a real card even though they look distinct in a `--config-dir` folder on
  a case-sensitive filesystem.
- **A setting whose own name is also a directory of other settings.**
  A card can't hold a file and a folder with the same name.

A name under `env/` is held to a second, tighter rule on top of the above,
since it has to work as a real environment variable your app reads with
`os.Getenv`: letters, digits and underscores only, not starting with a
digit. And `env/GOSD_*` is refused at build time — that's gosd-init's own
reserved namespace (see "App environment variables" in the runtime
contract) — with an actionable error naming the file to rename.

At **runtime**, the device doesn't refuse any of this — a card is edited by
hand, on whatever computer it happens to be plugged into, and those
computers write junk files unbidden. Instead `gosd-init` silently reads
*past* every name on this list (documentation, `.new`/`.unused` files, dot-
files, AppleDouble companions, `Thumbs.db`/`desktop.ini`) rather than
treating any of them as a setting, and a hand-written `env/GOSD_*` file is
logged and ignored rather than allowed to shadow gosd-init's own value.

## Padding is the reservation

A value file doesn't ship at exactly the length of its content. `gosd build`
pads every value out to at least 256 bytes with trailing newlines — so
`hostname` still reads as unset even though its file is 256 bytes long, not
zero. Shipping a value file **larger** than 256 bytes reserves that much
space instead: gosd's own `ingress/cloudflared/token` ships as 1024 bytes of
newlines, because a real Cloudflare Tunnel token needs more room than the
minimum.

That reservation is fixed the moment the image is built and can never grow
afterwards — it's what a provisioning tool overwrites, byte range for byte
range, when it splices a value into a downloaded `.img` (see [image
injection](image-injection.md#injecting-settings)). If your app's own
setting needs to hold something longer than 256 bytes, ship a longer file
from your `--config-dir` (a file of nothing but 4096 newlines reserves 4KiB
and still reads as unset). `.explain.md` files are never padded and are
never a target for injection — only value files reserve space.

## How the Imager wizard's answers land here

Raspberry Pi Imager's customization wizard doesn't write into `config/`
directly. It writes cloud-init's own `user-data`/`network-config` files
(the hostname and WiFi network you typed into the wizard) to the boot
partition, the same way it would for any cloud-init-aware OS. `gosd-init`
reads those files once, early in boot, and **consumes** them: it durably
deletes the cloud-init files first, and only then writes the hostname and
WiFi network they named into the ordinary `config/hostname` and
`config/wifi/*` files — exactly the files you'd have hand-edited yourself.

That's what makes a wizard-provisioned answer behave like any other setting
from then on: it's visible in the same file a person edits by hand, and it
survives a reflash the same way (see "Keeping your settings across a
reflash" below). A power cut in the narrow gap between the delete and the
write loses that boot's wizard answers — running the wizard again supplies
them again — but it can never happen the other way around: the deletion
always lands before anything is written, so a half-finished consumption can
never silently overwrite a value you'd already hand-edited on the card.

See [provisioning formats](provisioning-formats.md) for the full field
mapping this consumption implements.

## Keeping your settings across a reflash

Reflashing writes a whole new boot partition — `config/` included — so
without anything more, every value you'd typed onto a card would be
overwritten with whatever the new image shipped, every single time. An
image built with a data partition (`gosd build --data-size=expand`, or any
non-zero `--data-size`) avoids that: `gosd-init` keeps a copy of your
settings on `/data`, and puts them back onto a freshly flashed card
automatically.

**What counts as "your setting."** A value is kept the moment its file on
the card differs, byte for byte, from the value this image shipped it with
— an injected value, a hand-edit, or one the Imager wizard wrote in (see
above) all count equally. A file that still reads exactly as the image
shipped it is never kept, because there's nothing to distinguish it from a
value nobody ever touched.

**When it's restored.** Only on a boot where the running image's own
identity differs from the one the kept copy was last reconciled against —
i.e. the first boot after a genuine reflash to a different image. Flashing
the *same* image back over a card isn't treated as a reflash at all: every
value file returns to being byte-identical to what that image ships, which
is indistinguishable from someone having put every setting back to its
default by hand, and the store agrees — see "Putting a setting back to its
default" below.

**Per setting, on that boot:**

- **The card's own value always wins**, if it differs from what this image
  shipped. An injected value, or a hand-edit made before this exact boot, is
  the freshest statement of intent there is — the store never overwrites it.
- **Otherwise, the kept value is written back onto the card**, restoring
  what you had before the reflash. If this image's own shipped default for
  that setting is non-empty and different from what got restored, it also
  appears beside it as `<name>.new` — so you can see what the new release
  would have used by default without losing what you'd set. A `.new` file
  is purely informational: the device never reads one back as a setting.
  Nothing appears for a setting whose shipped default is empty (which is
  every secret-shaped setting gosd ships), since there's nothing useful to
  show.
- **A tunnel credential is never restored, because it was never kept.**
  `config/ingress/cloudflared/token` and
  `config/ingress/tailscale-funnel/authkey` are the one class of setting
  `gosd-init` refuses to copy to `/data` at all — see [why a reflash is not
  a reset](#a-reflash-is-not-a-factory-reset) below. Write yours onto the
  card again after reflashing, the same way you wrote it the first time.
- **A setting the new image no longer has a file for at all** is handed
  back onto the card as `<name>.unused` and then forgotten — you get exactly
  one reflash to notice and retrieve it before it's gone for good. The one
  exception is anything under `config/env/`: an app environment variable is
  your own namespace, never something baked into any particular image, so
  it's never treated as an orphan and survives reflash after reflash
  indefinitely.

**Putting a setting back to its default.** There's no separate "reset" step
— just put the file back exactly as the image shipped it (copy the value
from its `.new` sidecar, if one is sitting there, or empty the file for a
setting whose default is empty), or reflash the same image again. Either
way, the value stops being kept the next time the device reconciles it.

**Unsetting a kept `env/` value specifically:** empty the file rather than
deleting it. A deleted file is neither a kept value nor a forgotten one for
that boot — `gosd-init` can't tell "I meant to clear this" from "this file
just isn't here yet" — so an empty file is how you tell it you meant to
clear the setting.

**`data_flush` takes one extra boot to apply after a restore.** Whether
`/data` mounts with prompt writeback is decided *before* `/data` itself is
mounted — there's a chicken-and-egg problem in restoring that one setting
from the very partition it controls the mount options for — so a restored
`data_flush` value takes effect starting the boot *after* the one that
restored it, not the one that did the restoring.

## Secrets

A value like a WiFi passphrase or a tunnel token sits in **plaintext on the
boot FAT partition** — anyone who can put the card in a computer, or
download and mount the `.img`, can read it. There's no encryption. Put a
secret on the card only where that's an acceptable trade, and prefer a
credential you can revoke over one you can't.

The copy the store keeps on `/data` to survive a reflash (above) is the same
plaintext, just on a different partition. If you actually need to remove a
secret from a device, clearing `/data` (or reformatting it) is the operation
that does it; reflashing the boot partition alone does not. Every log line
the store or the tree ever writes names a *path*
(`config/ingress/cloudflared/token`), never the value that path holds.

### A reflash is not a factory reset

The data partition is untouched by reflashing — that is the whole point of
it, and it is what lets your settings come back — but it means `/data` is
**a place things survive the one remediation most people reach for**. GoSD
cannot tell who wrote what it finds there. The digest beside each kept value
proves the value was written *completely*; nothing proves it was written by
you. There is no key anywhere on these boards to prove it with: the boot
partition is erased by the very reflash the store exists to survive, `/data`
is the partition in question, and no board GoSD supports has a TPM or secure
element.

So take the restore for what it is. Anything with write access to `/data` —
someone who has had the card, or your own app, which runs as root and whose
storage `/data` is — can leave a setting there and have a freshly flashed
card pick it up. `gosd-init` narrows that as far as it honestly can:

- **Tunnel credentials are never kept and never restored.** Every other
  setting says what the device should *do*; a tunnel token or a tailnet
  authkey *is* the authorisation to reach the device, from anywhere. One of
  those coming back after a reflash would hand that reach to whoever left it
  there.
- **Restored values go through the same checks a hand-edited card does** —
  they are written onto the card and read back out of it, so there is no
  route into the device that skips the gates the card's own values pass.
- **A restore says so on the console**, naming the partition it came from.

If you need a device genuinely reset, clear or reformat the data partition.
On an ext4 `/data` (`gosd build --data-filesystem ext4`) that means a Linux
machine: macOS and Windows can neither read it nor clear it.
