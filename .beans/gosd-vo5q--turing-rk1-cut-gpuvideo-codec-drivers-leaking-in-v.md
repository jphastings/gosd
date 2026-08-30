---
# gosd-vo5q
title: 'Turing RK1: cut GPU/video-codec drivers leaking in via arm64 defconfig promotion'
status: todo
type: task
priority: normal
created_at: 2026-08-30T07:55:45Z
updated_at: 2026-08-30T20:28:03Z
parent: gosd-bntd
---

Real hardware bring-up (bean gosd-hycf, bench session 2026-08-30) showed rockchip-rga, hantro-vpu, and uvcvideo drivers probing in dmesg despite the kernel fragment's explicit '# CONFIG_DRM is not set' cut. This board has no display use case in scope for this epic, so these are dead weight (bigger image, slower boot) -- the same 'arm64 defconfig promotes =m to =y under the no-modules build' trap CLAUDE.md already documents for other boards in this fleet (mac80211_hwsim on the Pi Zero 2W, legacy gadget zoo, etc.). Find and explicitly cut the relevant CONFIG_ options (likely CONFIG_ROCKCHIP_RGA, CONFIG_VIDEO_HANTRO, CONFIG_USB_VIDEO_CLASS or similar -- verify exact symbol names against the pinned kernel tree, not from memory) in build/boards/turing-rk1/kernel/kernel-fragment.config, add them to kernelspec.go's ForbiddenY list, and confirm via a real gosd build-kernel run that they're gone from the resulting kernel.config.


## Deferred (2026-08-30, JP): don't cut yet, a display use case may come into scope

JP: leave this on the table for now -- the Turing Pi 2 has an HDMI port
that one of its nodes can drive, so a display use case for this board
isn't ruled out the way the original framing above assumed ("this board
has no display use case in scope for this epic, so these are dead
weight"). Cutting `rockchip-rga`/`hantro-vpu`/`CONFIG_DRM`-adjacent
drivers now would just have to be undone later if that materializes.
Not a blocker for `gosd-wf58`'s activation work either way -- the bloat is
image-size/boot-time cost, not a correctness issue. Revisit once a
display use case is confirmed in or out of scope, not before.
