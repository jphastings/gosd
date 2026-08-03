---
# gosd-fwrg
title: 'Hardware pass: boot a range-patched --placeholder image on the bench'
status: todo
type: task
created_at: 2026-08-03T21:17:53Z
updated_at: 2026-08-03T21:17:53Z
---

Deferred from gosd-49it (image injection): on the sdwire bench, flash an image built with --placeholder, patch a placeholder's manifest ranges with same-length bytes (no FAT tooling), boot it, and confirm the app reads the injected content at /boot/<path>. Then boot a pristine (unpatched) build and confirm the placeholder-as-absent behavior: a comment-only network-config is treated as no WiFi seed, and an app following the '# GOSD-PLACEHOLDER prefix means absent' rule gives its copy-a-config-onto-GOSD-BOOT guidance. Use the sdwire skill's flash/boot loop.
