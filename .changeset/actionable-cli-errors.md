---
gosd: minor
---

#### More actionable build errors, and a new `gadget` sentinel

`gosd build --data-size` now rejects a size that rounds down to less than
one sector (512 bytes) as soon as the flag is parsed, instead of running a
full cross-compile and artifact fetch for every board before failing deep
inside the image writer — the same "fail fast, before any image bytes
exist" contract the 256GiB ceiling check already gave.

A board package's own invariant-violation panic (for example, a u-boot.itb
too big for its locked offset) is now caught and turned into the same
single-line, actionable CLI error every other build failure produces,
instead of reaching the terminal as a raw Go stack trace.

`gadget` now exports `ErrNoController`, wrapped into the error `Apply`
returns when a board has no USB peripheral controller to bind to. This
matches the `errors.Is`-able sentinel convention `sound.ErrNoDevice`,
`emmc.ErrNoEMMC`, and `disk.ErrNoDisk` already give apps that want to
detect a missing device and degrade gracefully instead of failing outright.
