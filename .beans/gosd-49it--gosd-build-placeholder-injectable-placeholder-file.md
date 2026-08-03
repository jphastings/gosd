---
# gosd-49it
title: 'gosd build --placeholder: injectable placeholder files + .inject.json manifest'
status: in-progress
type: feature
created_at: 2026-08-03T20:45:01Z
updated_at: 2026-08-03T20:45:01Z
---

Implement the gosd side of the image-injection contract (handoff from atbackup's docs/IMAGE-INJECTION.md): `gosd build --placeholder <path>=<size>` pre-creates deterministic, comment-padded placeholder files on GOSD-BOOT and emits a `<image basename>.inject.json` sidecar recording the absolute byte ranges each placeholder's content occupies, so a browser (or any tool) can splice same-length config into a downloaded image without understanding FAT32.

## Why

A FAT file's content lives only in the data region; its size/location are recorded in the directory entry and FAT tables. A client that overwrites a placeholder's content ranges with exactly the same number of bytes changes the file without touching filesystem structures. Nothing at boot checksums GOSD-BOOT contents, so the patch is visible only to the programs that read those files. The downloading client needs no FAT code: verify hashes, splice padded content into declared ranges, save.

## Locked decisions (verified against the code 2026-08-03)

- **go-diskfs mechanics**: `(*fat12.File).GetDiskRanges` exists in v1.9.3 and returns coalesced, whole-cluster, **partition-relative** byte ranges (`dataStart + (cluster-2)*bytesPerCluster`, no `fs.start`). `fat32.FileSystem` embeds `*fat12.FileSystem`, so the file handle `image.Write`'s existing fat32 fs returns type-asserts to `interface{ GetDiskRanges() ([]fat12.DiskRange, error) }`. Absolute offset = range offset + `bootPartitionOffsetBytes` (16MiB, locked).
- **Clip to content length at the image layer**: GetDiskRanges returns whole clusters (Σ ≥ file size). `image.Write` validates Σ cluster lengths ≥ file size and every range inside partition 1, then clips the final range so `WriteReport.FileRanges` reports exact content ranges (Σ = file size). The manifest's "Σ length = size" invariant then holds by construction.
- **Placeholder contract v1**: rendered deterministically; exactly `size` bytes; final byte `\n`; first line `# GOSD-PLACEHOLDER v1 path=<path>`; then a short human explanation; then `#`-padding lines. Valid YAML, legible when the card is mounted. Consumers treat content still starting with `# GOSD-PLACEHOLDER` as absent.
- **No gosd runtime work**: a comment-only cloud-init `network-config` unmarshals to an empty YAML doc, which `internal/provision/networkconfig.go` already returns `nil, nil` for — pristine placeholder images behave exactly like images without the file.
- **New `internal/inject` package** owns the placeholder render + manifest types/writer, so pipeline (render into bootFiles) and cmd/gosd (manifest sidecar) share one implementation. Pipeline's option field is `Options.Placeholders []inject.Placeholder` (plan named it `PlaceholderSpec`; same shape `{Path, SizeBytes}`).
- **Sidecar naming** follows the catalog's `fragmentPath` convention: image extension replaced, i.e. `app-board.img` → `app-board.inject.json`, written beside the image whenever `--placeholder` was given.
- **Manifest schema** (`gosd_inject: 1`): `board`, `image{filename,size,sha256}` (sha256 of the whole pristine image), `placeholders[]{path,size,sha256,ranges[]{offset,length}}` — offsets absolute within the image; `ranges` ordered; content = concatenation of ranges; placeholder sha256 is over its rendered content.
- **Identity**: placeholders join `bootFiles` before the identity-hash loop in `pipeline.Assemble`, so they're covered by the image identity exactly like gosd.toml (deterministic render keeps `TestBuildIdentityIsReproducibleAcrossRebuilds` green).
- **Collisions**: refuse (case-insensitively — FAT is case-insensitive) a placeholder path colliding with any board boot file, `gosd.toml`, or another placeholder.

## Todos

- [ ] `internal/image`: `Spec.ReportRanges []string`, `ByteRange`, `WriteReport.FileRanges`; collect via GetDiskRanges after `writeBootFiles` while the fat32 handle is live; validate + clip; error if a ReportRanges path isn't a BootFiles key
- [ ] `internal/inject`: `Placeholder{Path,SizeBytes}`, deterministic `Render`, size/path validation, manifest types + `WriteManifest`
- [ ] `internal/pipeline`: `Options.Placeholders`, render into `bootFiles` before the identity loop, collision refusal, thread `ReportRanges` through to `image.Spec`
- [ ] `cmd/gosd/build.go`: repeatable `--placeholder <path>=<size>` flag (reuse `parseSizeBytes`), per-board `.inject.json` sidecar with whole-image sha256
- [ ] Tests: end-to-end proof (build → read manifest → `os.WriteAt` same-length bytes into reported ranges → `diskfs.Open` reads patched content back at FAT level); range/overlap/validation units; flag parsing; collision errors; identity reproducibility stays green
- [ ] Docs: `docs/image-injection.md` (contract + manifest format, pointer for the atbackup consumer), `--placeholder` help text
- [ ] Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`

## Deferred

- Hardware pass on the sdwire bench (boot a range-patched image; confirm app reads injected file at `/boot/<path>`; boot pristine and confirm placeholder-as-absent guidance) — follow-up when the bench is convenient.
