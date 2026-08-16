---
# gosd-uj4l
title: 'cubie-a5e: U-Boot''s USB scan costs ~4.5s of a 10.4s boot'
status: todo
type: task
created_at: 2026-08-16T19:35:26Z
updated_at: 2026-08-16T19:35:26Z
parent: gosd-h1wv
---

Measured during first hardware bring-up (bean gosd-6pfn). The cubie-a5e boots
in **10.38s** from SPL banner to app running (n=5, spread 0.15s), split:

| phase | time |
|---|---|
| SPL banner → `Starting kernel` (U-Boot) | 9.05s |
| kernel → gosd-init's first line | ~1.25s |
| gosd-init → /app running | 0.06s |

U-Boot is 87% of the boot, and within it the USB stack is the single biggest
item — `starting USB...` to the last `Bus usb@...: 1 USB Device(s) found` takes
**~4.5s**, scanning four controllers to conclude `0 Storage Device(s) found`:

```
[132.519] starting USB...
[137.063] Bus usb@4101000: 1 USB Device(s) found
...
[137.085]        scanning usb for storage devices... 0 Storage Device(s) found
```

We boot from mmc via extlinux and never from USB, so this scan buys nothing on
a gosd image. Dropping it (a defconfig fragment change, e.g. removing USB
storage/`usb_boot` from the boot targets, or `CONFIG_USB_STORAGE` off in
U-Boot only) should cut ~40% off the board's boot time.

Not done as part of bring-up because it changes a shipped artifact, so it needs
the artifacts release dance (see docs/artifacts.md) and should be measured
rather than assumed — and because the board has a blocking issue first
(gosd-84b8).

Worth checking whether the same scan is costing the other extlinux boards
(rock-4se, nanopi-zero2, radxa-zero-3e) the same 4.5s.

## Todos

- [ ] Confirm which U-Boot config controls the scan at our pin
- [ ] Rebuild, re-measure, and record the new baseline
- [ ] Check the other U-Boot boards for the same cost
