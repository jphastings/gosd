---
# gosd-c4rt
title: Bump the pinned Mozilla CA bundle snapshot
status: todo
type: task
priority: high
created_at: 2026-10-23T03:44:12Z
updated_at: 2026-10-23T03:44:12Z
---

Follow-up to gosd-x5c7, which moved gosd to Go 1.27.3 for the October 2026
crypto/x509 CA removal but couldn't bump the bundle: curl.se wasn't reachable
from that session.

`internal/cacerts.Pin` is still `cacert-2026-07-16.pem`, and every image's
initramfs ships it at `/etc/ssl/certs/ca-certificates.crt` (bean gosd-kzgq).
Go's fix covers intermediates, which the roots-only bundle never held, so
gosd-x5c7 doesn't depend on this. Still, the July snapshot predates Mozilla's
response to the same incident and three months of trust-store changes. An
app's HTTPS trusts exactly what this file says, and no image rebuilt from
today's gosd will see anything newer until the pin moves.

## Locked decisions

- Follow the package doc's bump procedure exactly: the newest **dated**
  `https://curl.se/ca/cacert-YYYY-MM-DD.pem`, never the rolling `cacert.pem`.
  Verify its sha256 against curl.se's own `<name>.sha256` for that snapshot,
  and move URL and SHA256 together.
- No test may fetch from curl.se: the build integration tests' network
  tripwire must stay green.
- The old snapshot's cache entry is pruned automatically (bean gosd-gdro), so
  there is no cache migration to write.

## Todo

- [ ] Pick the newest dated snapshot on https://curl.se/ca/ and record its
      date and sha256 here
- [ ] Diff it against `cacert-2026-07-16.pem` and note the added and removed
      roots in the Summary (check particularly for anything eMudhra-related)
- [ ] Update `internal/cacerts.Pin` (URL + SHA256)
- [ ] Clean-`HOME` `gosd build --board=qemu-virt` fetches and verifies the new
      bundle, and the built initramfs carries it
- [ ] `gosd: patch` change file naming the new snapshot date
