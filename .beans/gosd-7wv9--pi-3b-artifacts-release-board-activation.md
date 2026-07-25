---
# gosd-7wv9
title: 'Pi 3B: artifacts release + board activation'
status: todo
type: task
priority: normal
created_at: 2026-07-25T23:22:26Z
updated_at: 2026-07-25T23:22:42Z
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

- [ ] Verify package-and-release wiring covers pi-3b pre-tag (workflow_dispatch run)
- [ ] JP pushes the artifacts tag
- [ ] Activation PR (list above)
- [ ] Three-way verification recorded here: clean-HOME real-network all-boards
      build; offline dead-proxy cache re-run; content spot-check (e.g.
      kernel.config carries USB_DWCOTG/SMSC95XX =y and RUNTIME_UARTS=1;
      released tarball contains bcm2710-rpi-3-b.dtb)
