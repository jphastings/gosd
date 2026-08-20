---
gosd: patch
---

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
