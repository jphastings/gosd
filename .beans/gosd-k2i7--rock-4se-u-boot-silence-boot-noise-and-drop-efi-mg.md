---
# gosd-k2i7
title: 'rock-4se U-Boot: silence boot noise and drop efi_mgr detour'
status: todo
type: task
created_at: 2026-07-23T11:56:59Z
updated_at: 2026-07-23T11:56:59Z
---

Found during first real-hardware boot (gosd-sz6p, 2026-07-23). Serial log shows our v0.5.0 rock-4se U-Boot spending ~0.5 s and several scary-looking lines before reaching extlinux:

- Scans global bootmeth 'efi_mgr' FIRST: 'Cannot persist EFI variables without system partition', 'Loading Boot0000 mmc 1 failed', 'EFI boot manager: Cannot load any image', 'Boot failed (err=-14)' — then falls through to extlinux, which works. Disabling the EFI bootmeth (or reordering bootmeths so extlinux is tried first) removes the detour and the noise.
- 'Card did not respond to voltage select! : -110' twice (probing the empty eMMC slot).
- 'Reading from MMC(1)... *** Warning - bad CRC, using default environment' — we never save an env; CONFIG_ENV_IS_NOWHERE would make the default env intentional and silent.
- Also emits vidconsole cursor-position escape queries to serial.

All cosmetic/latency, not correctness — extlinux boot works. Likely applies to nanopi-zero2's U-Boot too (check its config when touching this). U-Boot config change = compiled-artifact change: tag-first release dance per docs/artifacts.md, no Version bump in the same PR. Boot-time context: full baseline table in gosd-sz6p (efi_mgr detour is ~0.46 s of a ~9.2 s boot).
