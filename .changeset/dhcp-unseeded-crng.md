---
gosd: patch
---

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
