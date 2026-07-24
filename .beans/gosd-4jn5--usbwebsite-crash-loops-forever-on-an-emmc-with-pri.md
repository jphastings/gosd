---
# gosd-4jn5
title: usbwebsite crash-loops forever on an eMMC with prior content
status: completed
type: bug
priority: normal
created_at: 2026-07-24T06:56:46Z
updated_at: 2026-07-24T07:44:16Z
---

Found during NanoPi Zero2 bring-up (gosd-odp7, 2026-07-24): examples/usbwebsite hardcodes emmc.FormatAndMount(label, mountpoint, false). On a board whose eMMC already holds other content (vendor image, prior project — the common case for real hardware, incl. FriendlyElec-shipped eMMC), the app prints the emmc package's 'refusing to reformat … pass destructive=true' error and exits 1, and gosd-init restart-loops it forever (observed: restart every ~1s, indefinitely). The docstring anticipates the no-eMMC case ('logs that plainly and exits') but not the dirty-eMMC case, and 'pass destructive=true' is developer-facing advice a user of the built example can't act on.

Fix direction (decide in this bean): an env-gated opt-in following the documented gosd.toml [env] pattern (like hello's GREETING) — e.g. WEBSITE_WIPE_EMMC=yes → destructive=true, so a user can consent by editing gosd.toml on the boot partition; plus exit cleanly/idle instead of crash-looping when consent is absent, with a log line pointing at the gosd.toml knob. Keep the safe default. Bench workaround used meanwhile: throwaway app calling emmc.FormatAndMount("WEBSITE", …, true) once to relabel the eMMC, after which stock usbwebsite accepts it non-destructively.

## Summary of Changes

- Added WEBSITE_WIPE_EMMC as a gosd.toml [env] consent knob (values 1/true/yes/on, case-insensitive; anything else, including unset, means no). Only the startup FormatAndMount call reads it; remount() keeps passing destructive=false always, per its docstring.
- Without consent, a "refusing to reformat" result no longer exits: usbwebsite logs the emmc error plus one actionable line naming the exact gosd.toml [env] fix, then idles instead of exiting, since gosd-init restarts exited apps regardless of exit code. Detection matches emmc's error text (no exported sentinel exists for this case; ErrNoEMMC is unaffected and still exits as before) - a documented, deliberate coupling.
- Idling uses a time.Sleep loop, not select {}: confirmed experimentally that a bare select {} with no other goroutine panics with "all goroutines are asleep - deadlock!" rather than blocking. The pre-existing select {} in the "computer attached, sharing as a drive" path had the same latent bug (no other goroutine keeps it alive either) and is fixed the same way as a drive-by, since it's the same file and same bug class.
- Updated the package docstring and examples/usbwebsite/README.md to document the dirty-eMMC case and the env var; docs/runtime.md only names hello's GREETING as its one illustrative app-env-var example (no per-example listing to keep in sync), so left unchanged.
- Added examples/usbwebsite/main_test.go (the example had no prior tests) covering the consent-parsing helper and the error-text match, per the "small test, no heavy harness" guidance.

Verified: go test ./..., go vet ./..., gofmt -l . (clean), golangci-lint run ./... and GOOS=linux golangci-lint run ./... (both clean), plus GOOS=linux GOARCH=arm64 and GOOS=linux GOARCH=arm GOARM=6 cross-builds of examples/usbwebsite.
