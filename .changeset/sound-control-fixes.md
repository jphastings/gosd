---
gosd: minor
---

#### The audibility pass no longer goes silent because of one bad control

`Open`'s audibility pass unmutes an ALSA codec that powers up muted — but
until now, one control write that failed (a mixer element this codec
doesn't have, a transient DAPM power race) aborted the whole pass, silently
skipping every later, unrelated unmute in the list. A board could end up
quieter than it should be for a reason that had nothing to do with the
control that actually failed.

The pass now attempts every change regardless of earlier failures, and
reports every failure together. `Device.Mixer().Changed` still lists what
succeeded; a caller that wants to know what didn't can check the error
`OpenWith` — or `Device`'s own `Mixer` — returns.

#### `SetControl` can now address a control at any index, with an actionable not-found error

`Device.SetControl` only ever matched `Control.Index == 0`, so a card with
more than one control sharing a name (real hardware — see `Control.Index`)
could only ever have the first one addressed, and the error read "no
control named %q" even when the name existed at a different index.

`Device`s from `Open`/`OpenWith` now also implement the new
`sound.IndexedControl` interface:

```go
if ic, ok := dev.(sound.IndexedControl); ok {
	err := ic.SetControlIndexed("Some Mixer Switch", 1, 1)
}
```

and the not-found error now says which index a matching name was actually
found at, when there is one, instead of reading like a typo'd name.
