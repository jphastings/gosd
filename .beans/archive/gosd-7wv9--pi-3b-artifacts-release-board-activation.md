---
# gosd-7wv9
title: 'Pi 3B: artifacts release + board activation'
status: completed
type: task
priority: normal
created_at: 2026-07-25T23:22:26Z
updated_at: 2026-07-26T12:55:00Z
parent: gosd-xhc3
blocked_by:
    - gosd-0nl7
---

The tag-first/bump-second dance per docs/artifacts.md, gosd-h8a8's shape.
Precondition: gosd-0nl7's kernel job is green on main and package-and-release
covers pi-3b. **Batching (epic locked decision, cross-ref gosd-36yy):** ride
the NEXT artifacts release window — most likely the fleet kernel bump waiting
on Linux v7.2.0 — rather than forcing a pi-3b-only release; if another
board's change tags a release sooner, join that one. JP pushes the
`artifacts/vX.Y.Z` tag; then ONE activation PR.

## Activation PR contents

- Flip pi-3b from RegisterInternal to boards.Register in cmd/gosd/build.go
- Bump internal/artifacts.Version to the new release
- internal/catalog: boardDisplayNames["pi-3b"] = "Raspberry Pi 3B";
  boardImagerDeviceTags["pi-3b"] — expect `["pi3-64bit"]` (the official
  "Raspberry Pi 3" device entry carries pi3-64bit/pi3-32bit, already
  documented at internal/catalog for pi-zero-2w's shared tag; our image is
  arm64 so pi3-64bit only). VERIFY against the live
  downloads.raspberrypi.org/os_list_imagingutility_v4.json at activation
  time and cite it (gosd-et0q's evidence pattern). Note the shared-namespace
  side effect: the pi-zero-2w entry already surfaces under "Raspberry Pi 3"
  in Imager, and the pi-3b entry will surface under "Raspberry Pi Zero 2 W" —
  document in the footnote like [^pi-tag] does.
- Regenerate the golden os_list.json; update catalog tests
- COMPATIBILITY.md: new Raspberry Pi 3B column + footnotes (Ethernet ✅ —
  the headline; USB gadget ➖ not-applicable, SoC USB hard-wired through the
  LAN9514 hub; WiFi ✅ Cypress 43430 blob set; code-complete-not-
  hardware-verified caveats until gosd-f5xm)
- CLAUDE.md Board IDs + arch lists; README board list;
  docs/board-build-tags.md internal-only note removed
- Update the no---board integration test to expect the pi-3b image among the
  public set; replace the catalog-writes-nothing test with a
  writes-entry test (gosd-h8a8 pattern)

## Todo

- [x] Verify package-and-release wiring covers pi-3b pre-tag (workflow_dispatch run) — superseded by the real release: `artifacts/v0.8.0` (published 2026-07-26T11:28:15Z) shipped `pi-3b.tar.zst` with all four expected files plus a manifest entry carrying kernel provenance (raspberrypi/linux @ 63598c83, the Pi fleet pin), proving gosd-0nl7's wiring end-to-end
- [x] JP pushes the artifacts tag — `artifacts/v0.8.0`, 2026-07-26
- [x] Activation PR (list above)
- [x] Three-way verification recorded here: clean-HOME real-network all-boards
      build; offline dead-proxy cache re-run; content spot-check (e.g.
      kernel.config carries USB_DWCOTG/SMSC95XX =y and RUNTIME_UARTS=1;
      released tarball contains bcm2710-rpi-3-b.dtb)

## Imager-tag live verification (2026-07-26)

Fetched `downloads.raspberrypi.org/os_list_imagingutility_v4.json` at
activation time (gosd-et0q's evidence pattern): the "Raspberry Pi 3" device
entry (description "Raspberry Pi 3 Model A+ / B / B+ and Compute Module
3 / 3+") carries tags `["pi3-64bit", "pi3-32bit"]` — exactly as expected.
GoSD's image is arm64, so `boardImagerDeviceTags["pi-3b"] = ["pi3-64bit"]`.
The device description also confirms the tag namespace covers both the 3B
and 3B+, matching the one-image family decision. Shared-namespace side
effect documented in `internal/catalog` and COMPATIBILITY.md's [^pi3b-tag]:
the "Raspberry Pi Zero 2 W" device carries the identical tags, so the pi-3b
entry also surfaces under "Raspberry Pi Zero 2 W", and vice versa.

## Verification evidence (2026-07-26)

Clean-machine acceptance run (fresh empty HOME under the session scratchpad,
no --board/--artifacts-dir, gosd built from this branch): `gosd build
./examples/hello` downloaded artifacts/v0.8.0's manifest.json and the six
requested board tarballs, sha256-verified them into
$HOME/Library/Caches/gosd/artifacts/v0.8.0/, and produced all SIX public
images — hello-pi-zero-2w.img, hello-pi-zero-w.img, **hello-pi-3b.img**,
hello-radxa-zero-3e.img, hello-nanopi-zero2.img, hello-rock-4se.img
(272 MiB each) — in 1m38s. qemu-virt correctly absent (internal); pi-3b
present in `--board`'s --help list.

Offline re-run (same fresh HOME, fresh output dir,
HTTP_PROXY/HTTPS_PROXY=http://127.0.0.1:9, GOPROXY=off): all six images
rebuilt in 49s with zero network — artifacts, Pi firmware blobs, and Go
modules all served from the caches the clean run populated.

Content spot-checks on the exact pi-3b files the clean run downloaded:
- kernel.config: `CONFIG_USB_LAN78XX=y` (line 3018 — the 3B+'s LAN7515,
  gosd-oq0z), `CONFIG_USB_NET_SMSC95XX=y` (3031 — the 3B's LAN9514),
  `CONFIG_SERIAL_8250_RUNTIME_UARTS=1` (3626 — the gosd-md4w lesson),
  `CONFIG_USB_DWCOTG=y` (4969 — the rpi-tree host controller driver);
  `# CONFIG_MAC80211_HWSIM is not set` (3323); legacy-gadget grep
  `grep -cE '^CONFIG_USB_(ZERO|ETH|GADGETFS|G_SERIAL|G_PRINTER|G_ACM_MS|G_MULTI|G_HID|CDC_COMPOSITE)=y'`
  → 0; `# CONFIG_USB_GADGET is not set` + `# CONFIG_USB_DWC2 is not set`
  (no UDC can exist — matches the board's structural no-gadget stance).
- The released pi-3b.tar.zst itself (independently downloaded and listed):
  exactly bcm2710-rpi-3-b-plus.dtb (35252 B), bcm2710-rpi-3-b.dtb
  (34731 B), kernel.config, kernel8.img (55470592 B) — BOTH family DTBs
  present, and both DTBs' sha256s match the manifest and the verified
  cache copies byte for byte.
- Release-wide: v0.8.0's manifest carries all seven boards; every non-pi-3b
  board's source pins/configs are identical to v0.7.0's (the two Pi Zero
  tarballs byte-identical; the Rockchip/qemu rebuilds differ only in
  non-reproducible binary output, same sources).

All quality gates green: go test ./..., go vet ./..., gofmt -l . (empty),
golangci-lint run ./... and GOOS=linux golangci-lint run ./... (0 issues
both).

## Summary of Changes

Branch `bean/gosd-7wv9-pi3b-activation`.

- cmd/gosd/build.go: `RegisterInternal` → `Register` for pi-3b — it joins
  the default all-boards build set, `--help`, and catalog generation.
- internal/artifacts.Version: v0.7.0 → v0.8.0, doc comment describing the
  payload (first pi-3b artifacts; everything else identical rebuilds).
- internal/catalog: `boardDisplayNames["pi-3b"] = "Raspberry Pi 3B"`;
  `boardImagerDeviceTags["pi-3b"] = ["pi3-64bit"]` with the live-verified
  evidence in the doc comment; fakeBuild + golden os_list.json regenerated
  for six boards; device-tag table test gains pi-3b.
- cmd/gosd/build_integration_test.go: default all-boards test now expects
  six images (qemu-virt back to being the only internal board);
  `TestBuildCatalogForPi3BOnlyWritesNothing` replaced by
  `TestBuildCatalogForPi3BWritesEntry` (the gosd-wskc/gosd-h8a8 pattern);
  stale internal-only comments updated.
- COMPATIBILITY.md: Raspberry Pi 3B column + six new footnotes
  ([^pi3b-family] one-image-covers-3B-and-3B+; [^pi3b-artifacts] v0.8.0
  provenance; [^pi3b-eth] Ethernet hardware-verified 2026-07-26 on a 3B+
  via lan78xx — the 3B's smsc95xx half is code-asserted only;
  [^pi3b-wifi] 43430 stack code-complete, 3B+'s BCM43455 blob gap called
  out; [^pi3b-tag] live-verified pi3-64bit; [^pi3b-no-gadget] hub-wired
  USB, structurally impossible); intro updated for six boards + the maiden
  boot; adjacent stale "next artifacts release" claims corrected to
  "shipped in artifacts/v0.7.0" (that pin landed in gosd-xo9u after the
  #123 refresh).
- CLAUDE.md Board IDs + arch lists; README board list;
  docs/board-build-tags.md internal-only note removed; docs/runtime.md
  GPIO/I2C/SPI tables gain the pi-3b rows (same header/pins as the Zeros)
  and two stale board-count phrases made count-agnostic.
