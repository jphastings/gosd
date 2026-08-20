---
gosd: patch
---

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
