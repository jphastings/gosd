---
# gosd-2j6z
title: 'Multi-arch builds: per-board GOARCH/GOARM'
status: todo
type: task
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-06T15:48:45Z
parent: gosd-ajpz
---

Keystone for pi-zero-w. Add an Arch concept to the Board interface/registry (e.g. Arch() returning {GOARCH, GOARM}; all existing boards return arm64). internal/build compiles the user app AND gosd-init once per distinct arch across the selected boards (cache per arch in the build dir; do not compile twice for two arm64 boards). The gosd-init module-cache path (internal/build/gosdinit.go) must pass the same env. CI smoke job adds a GOARM=6 cross-compile of examples/hello + cmd/gosd-init. Integration tests: fake board... keep it real — assert via the existing qemu-virt/pi builds that arm64 output is unchanged, plus a unit test that a GOARM=6 board yields an ARM 32-bit ELF (debug/elf: EM_ARM, ELFCLASS32).
- [ ] Board interface arch + all boards updated
- [ ] Per-arch compile pass with dedupe
- [ ] GOARM=6 static ELF verified by test
- [ ] CI smoke extended
