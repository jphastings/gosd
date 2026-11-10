---
# gosd-agtg
title: 'gosd docs: read the built-in docs in glow, and shorten build flag help'
status: completed
type: feature
priority: normal
created_at: 2026-11-10T09:22:12Z
updated_at: 2026-11-10T09:27:59Z
---

Add `gosd docs [name]`, which shows the top-level docs/*.md compiled into the binary using charmbracelet/glow's TUI (raw markdown when piped), and replace the verbose `gosd build` flag help with short copy that points at `gosd docs <name>`.

## Summary of Changes

- `gosd docs [name]` lists or shows the top-level `docs/*.md` embedded in the binary. On a terminal it opens charmbracelet/glow's TUI; otherwise it writes the raw Markdown. Logic lives in `internal/docview`; `cmd/gosd/docs.go` is wiring only.
- `gosd build` flag help is now short, with pointers of the form `gosd docs <name>`; a test checks every such pointer names a real doc.
- `gosd run`, `gosd build-kernel` and `gosd build-external` get the same short, sectioned flag help as `gosd build`.
- `gosd build --help` groups its flags under Flags, Runtime, User Experience, Go build and Advanced headings (cobra has no flag groups, so each section is its own FlagSet behind a custom usage func).
- glow re-reads its `Config.Path` from disk even when given content, so the doc is written to a temp dir and the process chdirs into it.
- `docs/docs.go` (package `docs`, exporting `FS`) is a necessary exception to the "everything else lives under `internal/`" public-API locked decision: `go:embed` cannot reach parent directories, so the embed has to sit beside the Markdown. It is not part of gosd's supported API.
- Adding glow required also bumping `charmbracelet/x/cellbuf` to v0.0.15 (v0.0.13 fails to compile against the `x/ansi` version already in the module graph). `flake.nix`'s `vendorHash` needs refreshing with nix.
