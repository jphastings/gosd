---
gosd: patch
---

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
