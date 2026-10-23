---
# gosd-c4rt
title: Bump the pinned Mozilla CA bundle snapshot
status: completed
type: task
priority: high
created_at: 2026-10-23T03:44:12Z
updated_at: 2026-10-23T07:37:58Z
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

- [x] Pick the newest dated snapshot on https://curl.se/ca/ and record its
      date and sha256 here
- [x] Diff it against `cacert-2026-07-16.pem` and note the added and removed
      roots in the Summary (check particularly for anything eMudhra-related)
- [x] Update `internal/cacerts.Pin` (URL + SHA256)
- [x] Clean-`HOME` `gosd build --board=qemu-virt` fetches and verifies the new
      bundle, and the built initramfs carries it
- [x] `gosd: patch` change file naming the new snapshot date


## Summary of Changes

Pinned `cacert-2026-10-20.pem` (newest dated snapshot on curl.se/ca), sha256 `796f1389bfea13c0ed78f663b765baaff54a508e5483786bd6ece73c5e3e3cbe`, matching curl.se's own `.sha256`. URL and SHA256 moved together in `internal/cacerts`; no test or fixture referenced the old snapshot.

Roots compared by certificate fingerprint against `cacert-2026-07-16.pem` (119 -> 118):

- Added: SECOM TLS RSA Root CA 2024, SECOM TLS ECC Root CA 2024, Telia EC TLS Root CA v3, Telia RSA TLS Root CA v3
- Removed: emSign ECC Root CA - C3, emSign Root CA - C1, emSign ECC Root CA - G3, ePKI Root Certification Authority, Atos TrustedRoot 2011

eMudhra finding: emSign is eMudhra's CA brand. Three of its four roots are gone from the store (C1, C3, ECC G3); only emSign Root CA - G1 remains. No root is labelled "eMudhra" literally.

Verified with a clean HOME: `gosd build --board=qemu-virt ./examples/hello` fetched the bundle into the fresh cache under the new digest, and the built image's initramfs `/etc/ssl/certs/ca-certificates.crt` hashes to the new sha256.
