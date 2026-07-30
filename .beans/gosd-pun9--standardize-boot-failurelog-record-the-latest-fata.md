---
# gosd-pun9
title: 'Standardize boot-failure.log: record the latest fatal boot issue on GOSD-BOOT for unattended devices'
status: todo
type: feature
created_at: 2026-07-30T21:11:39Z
updated_at: 2026-07-30T21:11:39Z
---

Direction from JP (2026-07-30, during gosd-6sac review): GoSD devices are unattended, so the latest run's fatal issue must be discernable without a serial cable — by pulling the card and reading `boot-failure.log` at the root of the GOSD-BOOT partition on any computer.

The seed of the pattern shipped with gosd-6sac: `boot.Deps.WriteBootFailure` (remount /boot read-write, overwrite boot-failure.log, sync, remount read-only — see `writeBootFailure` in cmd/gosd-init/internal/boot/platform_linux.go), currently used only by the data-corruption halt path. This bean is adopting it as the standard everywhere:

- [ ] Call WriteBootFailure from the general `fatal()` path for every fatal error that happens while /boot is mounted (boot-mount failures themselves obviously can't be recorded — enumerate which fatal paths can)
- [ ] Decide halt vs reboot per failure class: reboot for maybe-transient errors (current behaviour), halt for states no retry improves (the data-corruption path already halts). Record the rationale per class
- [ ] Define the file's format: latest failure only (overwrite), what context to include — gosd-init/app version, board, the error, actionable recovery steps. Note the clock usually reads 1970 at fatal time (no RTC, pre-NTP), so wall-clock timestamps are misleading; consider boot counts or uptime instead
- [ ] Document it in docs/runtime.md as a user-facing contract (and consider whether a healthy boot should DELETE a stale boot-failure.log — a device that recovered shouldn't look broken; decide and document)
- [ ] Consider whether the read-only-/data fallback for NON-expand images (mount failure of a fixed-size GOSD-DATA) should also record to boot-failure.log — today it degrades silently to EROFS, which is invisible on an unattended device

Risk note, decided acceptable for the corruption path: writing to GOSD-BOOT requires briefly remounting it read-write; a power cut during that window risks the boot FAT. Writes are tiny, rare (fatal paths only), and synced immediately.
