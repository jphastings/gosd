---
gosd: minor
---

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
