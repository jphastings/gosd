# ext4 golden image: maintainer regen recipe

Builds e2fsprogs from source and runs `mke2fs` inside Docker to (re)produce
`internal/diskfmt/ext4golden/golden.img.zst` -- the pristine ext4 filesystem
GoSD's `disk` package raw-copies onto internal drives and grows in place
with the kernel's online-resize ioctl. See that directory's `README.md` for
*why* every mke2fs parameter was chosen, the growth-ceiling proof, and the
asset's provenance; this file only covers running the recipe itself.

## Regenerate

```sh
./build.sh
```

Requires Docker (or a Docker-compatible context, e.g. colima) and `jq`. Not
part of any `go build`/`go test` run -- this is maintainer-only tooling, and
the only thing it changes is the checked-in asset + its manifest.

`build.sh` builds e2fsprogs from source at the pinned tag + commit (see the
top of the script), runs the exact `mke2fs` invocation recorded in
`Dockerfile`, verifies the result in a **privileged** container
(`verify.sh`: loop-mount, write, online `resize2fs` while mounted, fsck),
compresses with zstd, and writes the asset + `manifest.json` into
`../../internal/diskfmt/ext4golden/`.

## Pinned inputs

- **e2fsprogs**: built from source, not the distro package -- `E2FSPROGS_TAG`
  / `E2FSPROGS_COMMIT` in `build.sh`.
- **mke2fs parameters**: feature set and `-E`/`-J` flags are in
  `Dockerfile`'s `RUN mke2fs...` step; the fixed UUID/hash-seed/timestamp
  and journal/image sizes are variables at the top of `build.sh`.

## Why Docker, and why privileged verification is a separate step

`gosd build` itself never needs Docker (CLAUDE.md's carve-out list); this
recipe is maintainer-only tooling, run once per parameter change, mirroring
`build/boards/*/uboot`'s Docker-recipe shape. The verification step
loop-mounts a filesystem, which needs real root/`CAP_SYS_ADMIN` --
BuildKit's plain `RUN` steps can't do that (no `--privileged` support
without opting into the `security.insecure` entitlement), so it runs as a
separate `docker run --privileged` invocation against the same build-stage
image, not a `Dockerfile` instruction.
