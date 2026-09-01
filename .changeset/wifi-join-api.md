---
gosd: minor
---

#### New `wifi` package

`wifi.Join(ctx, wifi.Credentials{SSID, Passphrase}, wifi.Options{Persist:
bool})` lets an app join a WiFi network at runtime, using credentials it
obtained by its own means — an NFC tag, a provisioning screen, anything
other than what the config tree already had at boot. Join blocks until
gosd-init reports the attempt joined or failed. Off a device (any binary
not built by `gosd build`), Join returns an immediate, actionable error
rather than reaching for a filesystem that isn't there.
