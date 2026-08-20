---
gosd: minor
---

#### A reflash now resets a device's tunnel credentials

The settings `gosd-init` keeps on the data partition so they survive a
reflash have never been authenticated, and the data partition is the one
thing a reflash leaves alone. Anything able to write there — someone who has
had the card, or the app itself, which runs as root and whose storage
`/data` is — could leave a setting behind and have a freshly flashed card
pick it up. Reflashing is the most drastic remedy an owner has, and it did
not reach that copy at all.

There is no way to authenticate it: a key would have to live somewhere the
device can read and an attacker cannot, and on these boards there is nowhere
— the boot partition is erased by the very reflash the kept settings exist
to survive, `/data` is the partition in question, and no supported board has
a TPM or secure element. So rather than a check that only looks like one,
this release narrows what a kept setting can do:

- **Tunnel credentials are never kept, and so never restored.**
  `config/ingress/cloudflared/token` and
  `config/ingress/tailscale-funnel/authkey` no longer travel on the data
  partition, and any copy an earlier release left there is deleted on the
  next boot. Every other setting says what a device should *do*; a token or
  an authkey *is* the authorisation to reach it from anywhere, and one
  coming back after a reflash would hand that reach to whoever left it
  there. **If you reflash a device with a tunnel, write its token onto the
  new card** — the same act that put it there originally.
- **A restored hostname can no longer forge an `/etc/hosts` entry.** One
  carrying a newline would have added an attacker-chosen name-to-address
  mapping, which Go's resolver consults ahead of DNS for every lookup an app
  makes. The renderer now refuses any hostname that isn't one, whatever its
  caller believes, and app environment variable names restored onto a card
  are held to the same rule the build enforces.
- **A restore says so on the console**, naming the partition it came from.

Everything else is restored exactly as before. The trade-off is now written
down: a reflash is not a factory reset, and clearing or reformatting the
data partition is the operation that resets a device. See the config tree's
guide for the whole picture.
