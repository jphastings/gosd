---
gosd: patch
---

#### A crash report no longer carries the WiFi passphrase

`LAST_FATAL_ERROR.md` is a file whose own text asks its reader to forward the
whole thing, so gosd-init scrubs the secrets it knows about out of it first.
The WiFi passphrase was not one of them: every app environment value and both
ingress credentials were swept, and the one credential most likely to be
reused on another account was not.

It is now registered the same way and at the same moment as the tunnel
credentials — from both places one can come from, the card's `wifi/passphrase`
setting and the passphrase baked into the image — and appears in a report as
`{wifi: passphrase}`. The network's SSID is deliberately left alone: it is
broadcast to anyone in radio range, and removing it would cost a WiFi failure
the one detail that makes it diagnosable.

No gosd-init code path printed the passphrase before this, so no released
image is known to have leaked one. The point of the redaction rule set is that
nobody has to audit each new log line, or each upstream library's, to keep
that true.
