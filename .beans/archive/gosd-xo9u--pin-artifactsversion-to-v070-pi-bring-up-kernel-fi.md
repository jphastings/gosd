---
# gosd-xo9u
title: Pin artifacts.Version to v0.7.0 — Pi bring-up kernel fixes
status: completed
type: task
priority: normal
created_at: 2026-07-26T10:44:41Z
updated_at: 2026-07-26T10:49:28Z
---

Tag-first follow-up: artifacts/v0.7.0 published 2026-07-26 (all six board tarballs + manifest.json, verified via gh release view). Bump internal/artifacts.Version from v0.6.0 to v0.7.0 so stock builds pick up the Pi bring-up kernel fixes: pi-zero-w mini-UART registered at boot (gosd-md4w, CONFIG_SERIAL_8250_RUNTIME_UARTS=1), pi-zero-w SDIO controller enabled + hwsim phantom radios removed (gosd-6nl2, CONFIG_MMC_SDHCI_IPROC=y, no MAC80211_HWSIM), the Zero W VideoCore dma-ranges DTB patch (gosd-1ey5), the legacy-gadget eviction on the Pi boards (gosd-spjt), and the Pi 3B kernelspec (gosd-ypg1).

Three-way verification per docs/artifacts.md, recorded here:
- [x] Clean-machine build: fresh HOME, no --board/--artifacts-dir — all five public images (pi-zero-2w, pi-zero-w, radxa-zero-3e, nanopi-zero2, rock-4se) built from a real v0.7.0 download in 1m24s, 2026-07-26
- [x] Offline re-run: unreachable proxy (HTTP(S)_PROXY=http://127.0.0.1:9) + GOPROXY=off, same fresh HOME, fresh output dir — all five images rebuilt in 29s entirely from the cache the clean run populated, 2026-07-26
- [x] Content spot-check: downloaded v0.7.0 artifacts carry the Pi bring-up changes — all checks below pass against the clean run's cache, 2026-07-26

## Verification evidence (2026-07-26)

Clean-machine acceptance run (fresh HOME under the session scratchpad, no --board/--artifacts-dir, gosd built from this branch): gosd build ./examples/hello downloaded artifacts/v0.7.0's manifest.json and the five requested board tarballs, sha256-verified them into $HOME/Library/Caches/gosd/artifacts/v0.7.0/ (os.UserCacheDir confirmed to resolve under the fresh HOME), and produced all five public images — hello-pi-zero-2w.img, hello-pi-zero-w.img, hello-radxa-zero-3e.img, hello-nanopi-zero2.img, hello-rock-4se.img (272 MiB each) — in 1m24s. qemu-virt and pi-3b correctly absent (internal boards); the cached manifest lists six boards (incl. qemu-virt) but only the five requested tarballs were fetched.

Offline re-run (same fresh HOME, fresh output dir, HTTP_PROXY/HTTPS_PROXY=http://127.0.0.1:9, GOPROXY=off): all five images rebuilt in 29s with zero network — artifacts, Pi firmware blobs, and Go modules all served from the caches the clean run populated.

Content spot-checks on the exact files the clean run downloaded (the bytes this release exists to ship):
- pi-zero-w kernel.config: `CONFIG_SERIAL_8250_RUNTIME_UARTS=1` (line 2759 — gosd-md4w mini-UART at boot); `CONFIG_MMC_SDHCI_IPROC=y` (line 4263 — gosd-6nl2 SDIO controller); `# CONFIG_MAC80211_HWSIM is not set` (line 2463 — hwsim phantoms gone); legacy-gadget grep `grep -cE '^CONFIG_USB_(ZERO|ETH|GADGETFS|G_SERIAL|G_PRINTER|G_ACM_MS|G_MULTI|G_HID|CDC_COMPOSITE)=y'` → 0 (gosd-spjt zoo evicted).
- pi-zero-w bcm2835-rpi-zero-w.dtb: the 24-byte big-endian dma-ranges sequence 0x40000000,0x0,0x20000000,0x7e000000,0x20000000,0x02000000 found at byte offset 880 — BOTH VideoCore windows present (gosd-1ey5): bus 0x40000000→cpu 0x0 size 0x20000000, bus 0x7e000000→cpu 0x20000000 size 0x02000000.
- pi-zero-2w kernel.config: `# CONFIG_MAC80211_HWSIM is not set` (line 3324); same legacy-gadget grep → 0.

All quality gates green: go test ./... (46 packages ok), go vet ./..., gofmt -l . (empty), golangci-lint run ./... and GOOS=linux golangci-lint run ./... (0 issues both).

## Summary of Changes

Bumped internal/artifacts.Version from v0.6.0 to v0.7.0 in internal/artifacts/artifacts.go, replacing the stale v0.5.0 doc-comment paragraph with one describing v0.7.0's Pi bring-up payload (gosd-md4w mini-UART, gosd-6nl2 SDIO + hwsim removal, gosd-1ey5 dma-ranges DTB patch, gosd-spjt legacy-gadget eviction, gosd-ypg1 Pi 3B kernelspec; non-Pi boards unchanged rebuilds). No other Go file or doc names the pinned version as a current fact. Verified three ways per docs/artifacts.md, recorded above.
