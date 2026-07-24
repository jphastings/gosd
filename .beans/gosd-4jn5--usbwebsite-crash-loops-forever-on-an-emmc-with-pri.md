---
# gosd-4jn5
title: usbwebsite crash-loops forever on an eMMC with prior content
status: todo
type: bug
created_at: 2026-07-24T06:56:46Z
updated_at: 2026-07-24T06:56:46Z
---

Found during NanoPi Zero2 bring-up (gosd-odp7, 2026-07-24): examples/usbwebsite hardcodes emmc.FormatAndMount(label, mountpoint, false). On a board whose eMMC already holds other content (vendor image, prior project — the common case for real hardware, incl. FriendlyElec-shipped eMMC), the app prints the emmc package's 'refusing to reformat … pass destructive=true' error and exits 1, and gosd-init restart-loops it forever (observed: restart every ~1s, indefinitely). The docstring anticipates the no-eMMC case ('logs that plainly and exits') but not the dirty-eMMC case, and 'pass destructive=true' is developer-facing advice a user of the built example can't act on.

Fix direction (decide in this bean): an env-gated opt-in following the documented gosd.toml [env] pattern (like hello's GREETING) — e.g. WEBSITE_WIPE_EMMC=yes → destructive=true, so a user can consent by editing gosd.toml on the boot partition; plus exit cleanly/idle instead of crash-looping when consent is absent, with a log line pointing at the gosd.toml knob. Keep the safe default. Bench workaround used meanwhile: throwaway app calling emmc.FormatAndMount("WEBSITE", …, true) once to relabel the eMMC, after which stock usbwebsite accepts it non-destructively.
