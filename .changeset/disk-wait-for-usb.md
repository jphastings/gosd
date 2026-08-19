---
gosd: minor
---

#### `disk` can now wait for a USB drive that is still enumerating

`disk.FormatAndMount` discovers once. That is right for the storage GoSD was
first built around — an NVMe SSD or an onboard eMMC is on an on-SoC bus and is
already enumerated by the time an app's `main` runs — but USB mass storage is
not like that. A stick or an enclosure needs its hub port powered, then a probe,
then a scan, then a medium-ready report: commonly a second or two after the host
controller comes up, and longer through a hub or for a disk that spins up. An app
that reached `FormatAndMount` before all that finished got `ErrNoDisk` for a
drive that was physically plugged in.

The new `disk.Options.Wait` is how long to keep looking:

```go
res := <-disk.FormatAndMountWith("APPDATA", "/storage", disk.Options{
	Wait: 10 * time.Second,
})
```

Its zero value is what shipped before it existed, so no app changes behaviour by
upgrading. There is deliberately no default window: one would stall every app
that treats `ErrNoDisk` as "carry on without a disk", and every board with
nothing attached would pay it on each boot. A long `Wait` is also the honest way
to ask for "use a drive whenever someone plugs one in". `ErrNoDisk` now names the
option when an app never asked to wait, and reports how long it waited when it
did.
