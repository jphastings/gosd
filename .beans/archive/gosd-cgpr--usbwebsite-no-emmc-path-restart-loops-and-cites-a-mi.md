---
# gosd-cgpr
title: usbwebsite no-eMMC path restart-loops and cites a misleading example board
status: completed
type: task
priority: normal
created_at: 2026-07-24T15:54:00Z
updated_at: 2026-07-24T16:01:52Z
---

Two nits from real-hardware observations (gosd-odp7 NanoPi session + gosd-nlzf Zero 3E session, 2026-07-24):

1. usbwebsite's no-eMMC path prints its message and returns (exit 0), so gosd-init restart-loops it forever — pointless on a board that will never grow an eMMC mid-boot, and it spams the console every backoff interval (observed on both eMMC-less bench boards). The gosd-4jn5 fix gave the REFUSED-consent path an idleForever(); the ErrNoEMMC path should idle too (the docstring's 'logs that plainly and exits' predates the restart-loop understanding).

2. The message text 'this example needs one (e.g. a Radxa Zero 3E)' is misleading: eMMC is a build-to-order OPTION on the Zero 3E, and JP's two units both lack it — the message told a Zero 3E owner to get the board they were holding. Reword to name the requirement, not a board (e.g. 'needs a board with onboard eMMC fitted').

Small, self-contained; follow examples conventions (stdlib-only, cross-compile both arches).


## Summary of Changes

- examples/usbwebsite/main.go: the ErrNoEMMC branch now calls idleForever()
  instead of returning, so gosd-init stops restart-looping the app on a board
  that will never grow an eMMC mid-boot. Reworded the no-eMMC log line and the
  package docstring to name the requirement (onboard eMMC fitted) instead of
  a specific board.
- examples/usbwebsite/README.md: same rewording in the "Boards" section, plus
  a note that eMMC is a build-to-order option so having the right board model
  isn't the same as having eMMC fitted.
- No test changes: main_test.go only exercises isAffirmative, which is
  unaffected.
- Verified: go test ./..., go vet ./..., gofmt -l . (empty), golangci-lint
  run ./... and GOOS=linux golangci-lint run ./... (both clean), plus
  GOOS=linux GOARCH=arm64 and GOOS=linux GOARCH=arm GOARM=6 builds of
  examples/usbwebsite.
