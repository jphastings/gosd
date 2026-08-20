---
gosd: patch
---

#### `examples/usbwebsite` no longer publishes the device's own secrets

The example shared the SD card's data partition two ways that both handed out
more than the website. It served `/data` itself over HTTP — and `http.FileServer`
has no notion of a hidden file, so `http://<board>.local/.gosd/config/values/wifi/passphrase`
returned the WiFi passphrase to anyone on the network. It also offered that
same partition as a read-write USB drive, which gave any computer the cable
reached the passphrase, any ingress token and the Tailscale node's private
key, plus write access to the `/data/.gosd` area that survives a re-flash.

Both halves are now scoped to a directory the app owns. On the SD-card path
the site lives in a `website` folder and only that folder is served, and the
partition is no longer offered over USB unless the operator writes `yes` into
`config/env/WEBSITE_SHARE_DATA` — the app logs what that exposes when they do.
Boards with onboard eMMC are unchanged: that volume is the app's alone, so it
is still served from its root and shared freely.

If you run this example on an eMMC-less board, note that the USB drive no
longer appears by default, and that the site's files now belong in the
`website` folder of the data partition rather than at its root.

`gadget.MassStorage`'s documentation now says outright that a LUN is the whole
volume — there is no sharing a subdirectory — and that `ReadOnly`'s zero value
hands an unauthenticated host write access to all of it.
